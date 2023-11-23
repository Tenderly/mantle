package batchsubmitter

import (
	"context"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/tenderly/optimism/batch-submitter/drivers/proposer"
	"github.com/tenderly/optimism/batch-submitter/drivers/sequencer"
	tss "github.com/tenderly/optimism/batch-submitter/tss-client"
	bsscore "github.com/tenderly/optimism/bss-core"
	"github.com/tenderly/optimism/bss-core/dial"
	"github.com/tenderly/optimism/bss-core/metrics"
	"github.com/tenderly/optimism/bss-core/txmgr"

	"github.com/getsentry/sentry-go"
	"github.com/urfave/cli"
)

// Main is the entrypoint into the batch submitter service. This method returns
// a closure that executes the service and blocks until the service exits. The
// use of a closure allows the parameters bound to the top-level main package,
// e.g. GitVersion, to be captured and used once the function is executed.
func Main(gitVersion string) func(ctx *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		cfg, err := NewConfig(cliCtx)
		if err != nil {
			return err
		}

		// The call to defer is done here so that any errors logged from
		// this point on are posted to Sentry before exiting.
		if cfg.SentryEnable {
			defer sentry.Flush(2 * time.Second)
		}

		log.Info("Initializing batch submitter")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Set up our logging. If Sentry is enabled, we will use our custom log
		// handler that logs to stdout and forwards any error messages to Sentry
		// for collection. Otherwise, logs will only be posted to stdout.
		var logHandler log.Handler
		if cfg.SentryEnable {
			err := sentry.Init(sentry.ClientOptions{
				Dsn:              cfg.SentryDsn,
				Environment:      cfg.EthNetworkName,
				Release:          "batch-submitter@" + gitVersion,
				TracesSampleRate: bsscore.TraceRateToFloat64(cfg.SentryTraceRate),
				Debug:            false,
			})
			if err != nil {
				return err
			}

			logHandler = bsscore.SentryStreamHandler(os.Stdout, log.JSONFormat())
		} else if cfg.LogTerminal {
			logHandler = log.StreamHandler(os.Stdout, log.TerminalFormat(true))
		} else {
			logHandler = log.StreamHandler(os.Stdout, log.JSONFormat())
		}

		logLevel, err := log.LvlFromString(cfg.LogLevel)
		if err != nil {
			return err
		}

		log.Root().SetHandler(log.LvlFilterHandler(logLevel, logHandler))

		// Parse sequencer private key and CTC contract address.
		sequencerPrivKey, ctcAddress, err := bsscore.ParseWalletPrivKeyAndContractAddr(
			"Sequencer", cfg.Mnemonic, cfg.SequencerHDPath,
			cfg.SequencerPrivateKey, cfg.CTCAddress,
		)
		if err != nil {
			return err
		}

		// Parse proposer private key and SCC contract address.
		proposerPrivKey, sccAddress, err := bsscore.ParseWalletPrivKeyAndContractAddr(
			"Proposer", cfg.Mnemonic, cfg.ProposerHDPath,
			cfg.ProposerPrivateKey, cfg.SCCAddress,
		)
		if err != nil {
			return err
		}

		// Connect to L1 and L2 providers. Perform these last since they are the
		// most expensive.
		l1Client, err := dial.L1EthClientWithTimeout(ctx, cfg.L1EthRpc, cfg.DisableHTTP2)
		if err != nil {
			return err
		}

		l2Client, err := DialL2EthClientWithTimeout(ctx, cfg.L2EthRpc, cfg.DisableHTTP2)
		if err != nil {
			return err
		}

		tssClient, err := tss.NewClient(cfg.TssClientUrl, cfg.JwtSecret)
		if err != nil {
			return err
		}
		log.Info("Configured tss client", "url", cfg.TssClientUrl)

		if cfg.MetricsServerEnable {
			go metrics.RunServer(cfg.MetricsHostname, cfg.MetricsPort)
		}

		chainID, err := l1Client.ChainID(ctx)
		if err != nil {
			return err
		}

		txManagerConfig := txmgr.Config{
			ResubmissionTimeout:       cfg.ResubmissionTimeout,
			ReceiptQueryInterval:      time.Second,
			NumConfirmations:          cfg.NumConfirmations,
			SafeAbortNonceTooLowCount: cfg.SafeAbortNonceTooLowCount,
		}

		var services []*bsscore.Service
		if cfg.RunTxBatchSubmitter {
			batchTxDriver, err := sequencer.NewDriver(sequencer.Config{
				Name:                "Sequencer",
				L1Client:            l1Client,
				L2Client:            l2Client,
				BlockOffset:         cfg.BlockOffset,
				DaUpgradeBlock:      cfg.DaUpgradeBlock,
				DAAddr:              common.Address(common.HexToAddress(cfg.DAAddress)),
				CTCAddr:             ctcAddress,
				ChainID:             chainID,
				PrivKey:             sequencerPrivKey,
				EnableSequencerHsm:  cfg.EnableSequencerHsm,
				SequencerHsmAddress: cfg.SequencerHsmAddress,
				SequencerHsmAPIName: cfg.SequencerHsmAPIName,
				SequencerHsmCreden:  cfg.SequencerHsmCreden,
				BatchType:           sequencer.BatchTypeFromString(cfg.SequencerBatchType),
				MaxRollupTxn:        cfg.MaxRollupTxn,
				MinRollupTxn:        cfg.MinRollupTxn,
			})
			if err != nil {
				return err
			}

			services = append(services, bsscore.NewService(bsscore.ServiceConfig{
				Context:         ctx,
				Driver:          batchTxDriver,
				PollInterval:    cfg.PollInterval,
				ClearPendingTx:  cfg.ClearPendingTxs,
				L1Client:        l1Client,
				TxManagerConfig: txManagerConfig,
			}))
		}

		if cfg.RunStateBatchSubmitter {
			batchStateDriver, err := proposer.NewDriver(proposer.Config{
				Name:                        "Proposer",
				L1Client:                    l1Client,
				L2Client:                    l2Client,
				TssClient:                   tssClient,
				BlockOffset:                 cfg.BlockOffset,
				MinStateRootElements:        cfg.MinStateRootElements,
				MaxStateRootElements:        cfg.MaxStateRootElements,
				SCCAddr:                     sccAddress,
				CTCAddr:                     ctcAddress,
				FPRollupAddr:                common.HexToAddress(cfg.FPRollupAddress),
				ChainID:                     chainID,
				PrivKey:                     proposerPrivKey,
				SccRollback:                 cfg.EnableSccRollback,
				RollupTimeout:               cfg.RollupTimeout,
				PollInterval:                cfg.PollInterval,
				FinalityConfirmations:       cfg.FinalityConfirmations,
				EnableProposerHsm:           cfg.EnableProposerHsm,
				ProposerHsmAddress:          cfg.ProposerHsmAddress,
				ProposerHsmCreden:           cfg.ProposerHsmCreden,
				ProposerHsmAPIName:          cfg.ProposerHsmAPIName,
				AllowL2AutoRollback:         cfg.AllowL2AutoRollback,
				MinTimeoutStateRootElements: cfg.MinTimeoutStateRootElements,
			})
			if err != nil {
				return err
			}

			services = append(services, bsscore.NewService(bsscore.ServiceConfig{
				Context:         ctx,
				Driver:          batchStateDriver,
				PollInterval:    cfg.PollInterval,
				ClearPendingTx:  cfg.ClearPendingTxs,
				L1Client:        l1Client,
				TxManagerConfig: txManagerConfig,
			}))
		}

		batchSubmitter, err := bsscore.NewBatchSubmitter(ctx, cancel, services)
		if err != nil {
			log.Error("Unable to create batch submitter", "error", err)
			return err
		}

		log.Info("Starting batch submitter")
		log.Info("CloudHsm", "enable_sequencer", cfg.EnableSequencerHsm, "enable_proposer",
			cfg.EnableProposerHsm, "sequencer_address", cfg.SequencerHsmAddress, "proposer_address",
			cfg.ProposerHsmAddress)

		if err := batchSubmitter.Start(); err != nil {
			return err
		}
		defer batchSubmitter.Stop()

		log.Info("Batch submitter started")

		<-(chan struct{})(nil)

		return nil
	}
}
