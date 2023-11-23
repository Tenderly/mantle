package validator

import (
	"bytes"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/tenderly/mantle/fraud-proof/bindings"
	"github.com/tenderly/mantle/fraud-proof/metrics"
	"github.com/tenderly/mantle/fraud-proof/proof"
	"github.com/tenderly/mantle/fraud-proof/rollup/services"
	rollupTypes "github.com/tenderly/mantle/fraud-proof/rollup/types"
	"github.com/tenderly/mantle/l2geth/common"
	"github.com/tenderly/mantle/l2geth/core/rawdb"
	"github.com/tenderly/mantle/l2geth/log"
	"github.com/tenderly/mantle/l2geth/rlp"
	rpc2 "github.com/tenderly/mantle/l2geth/rpc"
)

func RegisterService(eth services.Backend, proofBackend proof.Backend, cfg *services.Config, auth *bind.TransactOpts) {
	validator, err := New(eth, proofBackend, cfg, auth)
	if err != nil {
		log.Crit("Failed to register the Rollup service", "err", err)
	}
	validator.Start()
	log.Info("Validator registered")
}

type ChallengeCtx struct {
	OpponentAssertion *rollupTypes.Assertion
	OurAssertion      *rollupTypes.Assertion
}

type Validator struct {
	*services.BaseService

	batchCh              chan *rollupTypes.TxBatch
	challengeCh          chan *ChallengeCtx
	challengeResoutionCh chan struct{}
}

func New(eth services.Backend, proofBackend proof.Backend, cfg *services.Config, auth *bind.TransactOpts) (*Validator, error) {
	base, err := services.NewBaseService(eth, proofBackend, cfg, auth)
	if err != nil {
		return nil, err
	}
	v := &Validator{
		BaseService:          base,
		batchCh:              make(chan *rollupTypes.TxBatch, 4096),
		challengeCh:          make(chan *ChallengeCtx),
		challengeResoutionCh: make(chan struct{}),
	}
	return v, nil
}

// This goroutine validates the assertion posted to L1 Rollup, advances
// stake if validated, or challenges if not
func (v *Validator) validationLoop() {
	defer v.Wg.Done()

	db := v.ProofBackend.ChainDb()

	// Listen to AssertionCreated event
	var assertionEventCh = make(chan *bindings.RollupAssertionCreated, 4096)
	assertionEventSub, err := v.Rollup.Contract.WatchAssertionCreated(&bind.WatchOpts{Context: v.Ctx}, assertionEventCh)
	if err != nil {
		log.Crit("Failed to watch rollup event", "err", err)
	}
	defer assertionEventSub.Unsubscribe()

	isInChallenge := false
	challengeCtxEnc := rawdb.ReadFPValidatorChallengeCtx(db)
	if challengeCtxEnc != nil {
		isInChallenge = true
	}

	for {
		stakerAddr, err := v.Rollup.Registers(v.TransactOpts.From)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		stakerStatus, err := v.Rollup.Stakers(stakerAddr)
		if err != nil {
			log.Error("UNHANDELED: Can't find stake, validator state corrupted", "err", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if isInChallenge {
			// Wait for the challenge resolution
			select {
			case <-v.challengeResoutionCh:
				log.Info("Validator finished challenge, reset isInChallenge status")
				isInChallenge = false
				rawdb.DeleteFPValidatorChallengeCtx(db)
			case <-v.Ctx.Done():
				return
			}
		} else {
			select {
			case ev := <-assertionEventCh:
				metrics.Metrics.MustGetGaugeVec(metrics.NameIndex.Name()).
					WithLabelValues(metrics.NameIndex.LabelAssertionIndex()).Set(float64(ev.AssertionID.Uint64()))
				metrics.Metrics.MustGetGaugeVec(metrics.NameSize.Name()).
					WithLabelValues(metrics.NameSize.LabelAssertionSize()).Set(float64(ev.InboxSize.Uint64()))
				// only one verifier and check it
				// check challenge status
				if !bytes.Equal(stakerStatus.CurrentChallenge.Bytes(), common.BigToAddress(common.Big0).Bytes()) {
					continue
				}
				// check staker status
				zombies, _ := v.Rollup.Zombies(big.NewInt(0))
				if !bytes.Equal(zombies.StakerAddress.Bytes(), common.BigToAddress(common.Big0).Bytes()) {
					continue
				}

				if common.Address(ev.AsserterAddr) == v.Config.StakeAddr {
					log.Info("Stake address", "address", v.Config.StakeAddr)
					// Create by our own for challenge
					continue
				}
				// New assertion created on Rollup
				log.Info("Validator get new assertion, check it with local block....")
				log.Info("check ev.AssertionID....", "id", ev.AssertionID)

				startID := stakerStatus.AssertionID.Uint64()
				// advance the assertion that has fallen behind
				for ; startID < ev.AssertionID.Uint64(); startID++ {
					checkID := startID + 1
					assertion, err := v.AssertionMap.Assertions(new(big.Int).SetUint64(checkID))
					if err != nil {
						log.Error("Validator get assertion failed", "assertionID", checkID, "err", err)
						time.Sleep(5 * time.Second)
						break
					}
					if assertion.InboxSize.Uint64() == 0 {
						// Skip assertions that have been deleted
						continue
					}
					checkAssertion := &rollupTypes.Assertion{
						ID:        new(big.Int).SetUint64(checkID),
						VmHash:    assertion.StateHash,
						InboxSize: assertion.InboxSize,
						Parent:    assertion.Parent,
					}
					block, err := v.BaseService.ProofBackend.BlockByNumber(v.Ctx, rpc2.BlockNumber(checkAssertion.InboxSize.Int64()))
					if err != nil {
						log.Error("Validator get block failed", "err", err)
						break
					}
					if block == nil {
						log.Error("Validator get block is nil, sleep for a while")
						time.Sleep(5 * time.Second)
						break
					}
					if bytes.Compare(checkAssertion.VmHash.Bytes(), block.Root().Bytes()) != 0 {
						//  Validation failed
						log.Info("Validator check assertion vmHash failed, start challenge assertion....")
						ourAssertion := &rollupTypes.Assertion{
							VmHash:    block.Root(),
							InboxSize: checkAssertion.InboxSize,
							Parent:    assertion.Parent,
						}
						challengeCtx := ChallengeCtx{checkAssertion, ourAssertion}
						data, _ := rlp.EncodeToBytes(challengeCtx)
						rawdb.WriteFPValidatorChallengeCtx(db, data)

						v.challengeCh <- &challengeCtx
						isInChallenge = true
						break
					} else {
						// Validation succeeded, confirm assertion and advance stake
						log.Info("Validator advance stake into assertion", "ID", ev.AssertionID, "now", startID)
						// todo ：During frequent interactions, it is necessary to check the results of the previous interaction
						tx, err := v.Rollup.AdvanceStake(new(big.Int).SetUint64(startID + 1))
						if err != nil {
							log.Error("UNHANDELED: Can't advance stake, validator state corrupted", "err", err)
							break
						}
						metrics.Metrics.MustGetGaugeVec(metrics.NameFee.Name()).
							WithLabelValues(metrics.NameFee.LabelValidatorVerifyFee()).Set(float64(tx.Cost().Uint64()))
						balance, err := v.BaseService.L1.BalanceAt(v.Ctx, v.Rollup.TransactOpts.From, nil)
						if err == nil {
							metrics.Metrics.MustGetGaugeVec(metrics.NameBalance.Name()).
								WithLabelValues(metrics.NameBalance.LabelValidatorBalance()).Set(float64(balance.Uint64()))
						}
					}
					metrics.Metrics.MustGetGaugeVec(metrics.NameIndex.Name()).
						WithLabelValues(metrics.NameIndex.LabelVerifiedIndex()).Set(float64(checkID))
				}
			case <-v.Ctx.Done():
				return
			}
		}
	}
}

func (v *Validator) challengeLoop() {
	defer v.Wg.Done()

	// challenge position
	// 1.Assertion create
	// 1.1 already created and now at correct assertion position
	// 1.2 need create new assertion then create challenge by ctx.inboxSize and ctx.vmHash

	// 2.Challenge create
	// 2.1 already created challenge
	// 2.2 challenge need create by seq.assertionID and val.assertion.

	// 3.Bisected execute
	// 3.1 sub channels and get current bisected
	// 3.2 already finished challenge

	// Watch AssertionCreated event
	var createdCh = make(chan *bindings.RollupAssertionCreated, 4096)
	createdSub, err := v.Rollup.Contract.WatchAssertionCreated(&bind.WatchOpts{Context: v.Ctx}, createdCh)
	if err != nil {
		log.Crit("Failed to watch rollup event", "err", err)
	}
	defer createdSub.Unsubscribe()

	var challengedCh = make(chan *bindings.RollupAssertionChallenged, 4096)
	challengedSub, err := v.Rollup.Contract.WatchAssertionChallenged(&bind.WatchOpts{Context: v.Ctx}, challengedCh)
	if err != nil {
		log.Crit("Failed to watch rollup event", "err", err)
	}
	defer challengedSub.Unsubscribe()

	// Watch L1 blockchain for challenge timeout
	var headCh = make(chan *ethtypes.Header, 4096)
	headSub, err := v.L1.SubscribeNewHead(v.Ctx, headCh)
	if err != nil {
		log.Crit("Failed to watch l1 chain head", "err", err)
	}
	defer headSub.Unsubscribe()

	var challengeSession *bindings.ChallengeSession
	var states []*proof.ExecutionState

	var bisectedCh = make(chan *bindings.ChallengeBisected, 4096)
	var bisectedSub event.Subscription
	var challengeCompletedCh = make(chan *bindings.ChallengeChallengeCompleted, 4096)
	var challengeCompletedSub event.Subscription

	restart := false
	inChallenge := false
	var ctx *ChallengeCtx
	var opponentTimeoutBlock uint64

	db := v.ProofBackend.ChainDb()

	go func() {
		// The necessity of local storage:
		// Can't judge whether the interruption has just entered the challenge process and did not create assertions
		challengeCtxEnc := rawdb.ReadFPValidatorChallengeCtx(db)
		if challengeCtxEnc != nil {
			// Before the program was exited last time, it had
			// entered the challenge state and did not execute it to challenge complete.
			// we need to re-enter in the challenge process.
			// Find the entry point through the state of the L1.
			stakerAddr, err := v.Rollup.Registers(v.TransactOpts.From)
			if err != nil {
				log.Error("get operator register error", "operator", v.TransactOpts.From, "err", err)
				return
			}
			stakeStatus, err := v.Rollup.Stakers(stakerAddr)
			if err != nil {
				log.Error("get staker error", "addr", stakerAddr, "err", err)
				return
			}
			currentAssertion, err := v.AssertionMap.Assertions(stakeStatus.AssertionID)
			if err != nil {
				log.Error("get assertion error", "assertionID", stakeStatus.AssertionID, "err", err)
				return
			}
			var challengeCtx ChallengeCtx
			if err = rlp.DecodeBytes(challengeCtxEnc, &challengeCtx); err != nil {
				log.Error("decode challengeCtx error", "err", err)
				return
			}
			ctx = &challengeCtx
			challengeContext, _ := v.Rollup.ChallengeCtx()

			if currentAssertion.InboxSize.Cmp(challengeCtx.OurAssertion.InboxSize) < 0 &&
				!bytes.Equal(currentAssertion.StateHash[:], challengeCtx.OurAssertion.VmHash[:]) {
				// did not create assertion
				v.challengeCh <- &challengeCtx
				log.Info("Did not create assertion")
			} else if bytes.Equal(stakeStatus.CurrentChallenge.Bytes(), common.BigToAddress(common.Big0).Bytes()) {
				// did not create challenge
				createdCh <- &bindings.RollupAssertionCreated{
					AssertionID:  stakeStatus.AssertionID,
					AsserterAddr: v.Rollup.TransactOpts.From,
					VmHash:       currentAssertion.StateHash,
					InboxSize:    currentAssertion.InboxSize,
				}
			} else if challengeContext.Completed {
				// already challenged do nothing
				v.challengeResoutionCh <- struct{}{}
				log.Info("Challenge already completed")
			} else {
				// in bisectedCh
				challengedCh <- &bindings.RollupAssertionChallenged{
					AssertionID:   ctx.OpponentAssertion.ID,
					ChallengeAddr: stakeStatus.CurrentChallenge,
				}
				restart = true
			}
		}
	}()

	for {
		if inChallenge {
			select {
			case ev := <-bisectedCh:
				// case get bisection, if is our turn
				//   1. In single step, submit proof
				//   2. In multiple step, track current segment, update
				log.Info("Validator saw new bisection coming...")
				responder, err := challengeSession.CurrentResponder()
				if err != nil {
					// TODO: error handling
					log.Error("Can not get current responder", "error", err)
					continue
				}
				// If it's our turn
				log.Info("Responder info...", "responder", responder, "staker", v.Config.StakeAddr)
				if common.Address(responder) == v.Config.StakeAddr {
					log.Info("Validator start to respond new bisection...")
					err := services.RespondBisection(v.BaseService, challengeSession, ev, states)
					if err != nil {
						// TODO: error handling
						log.Error("Can not respond to bisection", "error", err)
						continue
					}
				} else {
					log.Info("Validator check bisection respond time left...")
					opponentTimeLeft, err := challengeSession.CurrentResponderTimeLeft()
					if err != nil {
						// TODO: error handling
						log.Error("Can not get current responder left time", "error", err)
						continue
					}
					log.Info("[challenge] Opponent time left", "time", opponentTimeLeft)
					opponentTimeoutBlock = ev.Raw.BlockNumber + opponentTimeLeft.Uint64()
				}
			case header := <-headCh:
				if opponentTimeoutBlock == 0 {
					continue
				}
				// TODO: can we use >= here?
				if header.Number.Uint64() > opponentTimeoutBlock {
					_, err := challengeSession.Timeout()
					if err != nil {
						log.Error("Can not timeout opponent", "error", err)
						continue
						// TODO: wait some time before retry
						// TODO: fix race condition
					}
				}
			case ev := <-challengeCompletedCh:
				// TODO: handle if we are not winner --> state corrupted
				log.Info("[challenge] Challenge completed", "winner", ev.Winner)
				bisectedSub.Unsubscribe()
				challengeCompletedSub.Unsubscribe()
				states = []*proof.ExecutionState{}
				inChallenge = false
				v.challengeResoutionCh <- struct{}{}
			case <-v.Ctx.Done():
				bisectedSub.Unsubscribe()
				challengeCompletedSub.Unsubscribe()
				return
			}
		} else {
			select {
			case ctx = <-v.challengeCh:
				log.Info("Validator get challenge context, create challenge assertion")
				metrics.Metrics.MustGetCounterVec(metrics.NameAlert.Name()).
					WithLabelValues(metrics.NameAlert.LabelAlertChallengeStart()).Inc()

				_, err = v.Rollup.CreateAssertion(
					ctx.OurAssertion.VmHash,
					ctx.OurAssertion.InboxSize,
				)
				if err != nil {
					log.Error("UNHANDELED: Can't create assertion for challenge, validator state corrupted", "err", err)
					v.challengeCh <- ctx
					continue
				}
			case ev := <-createdCh:
				if common.Address(ev.AsserterAddr) == v.Config.StakeAddr {
					if ev.VmHash == ctx.OurAssertion.VmHash {
						log.Info("Assertion ID", "opponentAssertion.ID", ctx.OpponentAssertion.ID, "ev.AssertionID", ev.AssertionID)
						_, err := v.Rollup.ChallengeAssertion(
							[2]ethcommon.Address{
								ethcommon.Address(v.Config.SequencerAddr),
								ethcommon.Address(v.Config.StakeAddr),
							},
							[2]*big.Int{
								ctx.OpponentAssertion.ID,
								ev.AssertionID,
							},
						)
						if err != nil {
							log.Error("UNHANDELED: Can't start challenge, validator state corrupted", "err", err)
							createdCh <- ev
							continue
						}
					}
				}
			case ev := <-challengedCh:
				if ctx == nil {
					continue
				}
				log.Info("Validator saw new challenge", "assertion id", ev.AssertionID, "expected id", ctx.OpponentAssertion.ID, "block", ev.Raw.BlockNumber)
				if ev.AssertionID.Cmp(ctx.OpponentAssertion.ID) == 0 {
					challenge, err := bindings.NewChallenge(ev.ChallengeAddr, v.L1)
					if err != nil {
						log.Error("Failed to access ongoing challenge", "address", ev.ChallengeAddr, "err", err)
						challengedCh <- ev
						continue
					}
					challengeSession = &bindings.ChallengeSession{
						Contract:     challenge,
						CallOpts:     bind.CallOpts{Pending: true, Context: v.Ctx},
						TransactOpts: *v.TransactOpts,
					}
					bisectedSub, err = challenge.WatchBisected(&bind.WatchOpts{Context: v.Ctx}, bisectedCh)
					if err != nil {
						log.Error("Failed to watch challenge event", "err", err)
						challengedCh <- ev
						continue
					}
					challengeCompletedSub, err = challenge.WatchChallengeCompleted(&bind.WatchOpts{Context: v.Ctx}, challengeCompletedCh)
					if err != nil {
						log.Error("Failed to watch challenge event", "err", err)
						challengedCh <- ev
						continue
					}
					parentAssertion, err := ctx.OurAssertion.GetParentAssertion(v.AssertionMap)
					if err != nil {
						log.Error("Failed to watch challenge event", "err", err)
						challengedCh <- ev
						continue
					}
					log.Info("Validator start to GenerateStates", "parentAssertion.InboxSize", parentAssertion.InboxSize.Uint64(), "ctx.ourAssertion.InboxSize", ctx.OurAssertion.InboxSize.Uint64())
					states, err = proof.GenerateStates(
						v.ProofBackend,
						v.Ctx,
						parentAssertion.InboxSize.Uint64(),
						ctx.OurAssertion.InboxSize.Uint64(),
						nil,
					)
					log.Info("Validator generate states end...")
					if err != nil {
						log.Error("Failed to generate states", "err", err)
						challengedCh <- ev
						continue
					}
					log.Info("Print generated states", "states[0]", states[0].Hash().String(), "states[numSteps]", states[len(states)-1].Hash().String())

					if restart {
						curr, err := challengeSession.CurrentBisected()
						if err != nil {
							log.Error("Failed to get current bisected", "err", err)
							challengedCh <- ev
							continue
						}
						bisectedCh <- &bindings.ChallengeBisected{
							StartState:              curr.StartState,
							MidState:                curr.MidState,
							EndState:                curr.EndState,
							BlockNum:                curr.BlockNum,
							BlockTime:               curr.BlockTime,
							ChallengedSegmentStart:  curr.ChallengedSegmentStart,
							ChallengedSegmentLength: curr.ChallengedSegmentLength,
						}
						restart = false
					}

					inChallenge = true
				}
			case <-headCh:
				continue // consume channel values
			case <-v.Ctx.Done():
				return
			}
		}
	}
}

func (v *Validator) Start() error {
	//genesis := v.BaseService.Start(true, true)

	v.Wg.Add(2)
	go v.validationLoop()
	go v.challengeLoop()

	if len(os.Getenv("FP_METRICS_SERVER_ENABLE")) > 0 {
		port, ok := os.LookupEnv("FP_METRICS_PORT")
		if !ok {
			port = "9190"
		}
		host, ok := os.LookupEnv("FP_METRICS_HOSTNAME")
		if !ok {
			host = "0.0.0.0"
		}
		go metrics.Metrics.Start("fp-validator", host, port)
	}

	log.Info("Validator started")
	return nil
}

func (v *Validator) Stop() error {
	log.Info("Validator stopped")
	v.Cancel()
	v.Wg.Wait()
	return nil
}

func (v *Validator) APIs() []rpc.API {
	// TODO: validator APIs
	return []rpc.API{}
}
