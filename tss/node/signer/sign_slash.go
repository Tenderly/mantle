package signer

import (
	"encoding/json"
	"errors"
	"strings"

	ethc "github.com/ethereum/go-ethereum/common"

	tsscommon "github.com/tenderly/mantle/tss/common"
	tdtypes "github.com/tendermint/tendermint/rpc/jsonrpc/types"
)

const (
	slashingMethodName = "slashing"
)

func (p *Processor) SignSlash() {
	defer p.wg.Done()
	logger := p.logger.With().Str("step", "sign Slash Message").Logger()

	logger.Info().Msg("start to sign Slash message ")

	go func() {
		defer func() {
			logger.Info().Msg("exit sign process")
		}()
		for {
			select {
			case <-p.stopChan:
				return
			case req := <-p.signSlashChan:
				var resId = req.ID.(tdtypes.JSONRPCStringID).String()
				logger.Info().Msgf("dealing resId (%s) ", resId)

				var nodeSignRequest tsscommon.NodeSignRequest
				rawMsg := json.RawMessage{}
				nodeSignRequest.RequestBody = &rawMsg

				if err := json.Unmarshal(req.Params, &nodeSignRequest); err != nil {
					logger.Error().Msg("failed to unmarshal node sign request")
					RpcResponse := tdtypes.NewRPCErrorResponse(req.ID, 201, "failed", err.Error())
					if err := p.wsClient.SendMsg(RpcResponse); err != nil {
						logger.Error().Err(err).Msg("failed to send msg to manager")
					}
					continue
				}
				var requestBody tsscommon.SlashRequest
				if err := json.Unmarshal(rawMsg, &requestBody); err != nil {
					logger.Error().Msg("failed to umarshal slash params request body")
					RpcResponse := tdtypes.NewRPCErrorResponse(req.ID, 201, "failed", err.Error())
					if err := p.wsClient.SendMsg(RpcResponse); err != nil {
						logger.Error().Err(err).Msg("failed to send msg to manager")
					}
					continue
				}
				nodeSignRequest.RequestBody = requestBody

				err := p.checkSlashMessages(requestBody)
				if err != nil {
					RpcResponse := tdtypes.NewRPCErrorResponse(req.ID, 201, "failed", err.Error())

					if err := p.wsClient.SendMsg(RpcResponse); err != nil {
						logger.Error().Err(err).Msg("failed to send msg to manager")
					}
					logger.Err(err).Msg("check event failed")
					continue
				}

				nodesaddrs := make([]ethc.Address, len(nodeSignRequest.Nodes))
				for i, node := range nodeSignRequest.Nodes {
					addr, _ := tsscommon.NodeToAddress(node)
					nodesaddrs[i] = addr
				}
				hashTx, err := tsscommon.SlashMsgHash(requestBody.BatchIndex, requestBody.Address, nodesaddrs, requestBody.SignType)
				if err != nil {
					logger.Err(err).Msg("failed to encode SlashMsg")
					RpcResponse := tdtypes.NewRPCErrorResponse(req.ID, 201, "failed", err.Error())
					if err := p.wsClient.SendMsg(RpcResponse); err != nil {
						logger.Error().Err(err).Msg("failed to send msg to manager")
					}
					continue
				}

				data, culprits, err := p.handleSign(nodeSignRequest, hashTx, logger)

				if err != nil {
					logger.Error().Msgf("slash %s sign failed ", requestBody.Address)
					var errorRes tdtypes.RPCResponse
					if len(culprits) > 0 {
						respData := strings.Join(culprits, ",")
						errorRes = tdtypes.NewRPCErrorResponse(req.ID, 100, err.Error(), respData)
						p.nodeStore.AddCulprits(culprits)
					} else {
						errorRes = tdtypes.NewRPCErrorResponse(req.ID, 201, "sign failed", err.Error())
					}
					er := p.wsClient.SendMsg(errorRes)
					if er != nil {
						logger.Err(er).Msg("failed to send msg to tss manager")
					} else {
						p.removeWaitSlashMsg(requestBody)
					}
					continue
				}

				signResponse := tsscommon.SignResponse{
					Signature: data,
				}

				RpcResponse := tdtypes.NewRPCSuccessResponse(req.ID, signResponse)
				err = p.wsClient.SendMsg(RpcResponse)
				if err != nil {
					logger.Err(err).Msg("failed to sendMsg to bridge ")
				} else {
					logger.Info().Msg("send slash sign response successfully")
					p.removeWaitSlashMsg(requestBody)
				}
			}
		}
	}()
}

func (p *Processor) checkSlashMessages(sign tsscommon.SlashRequest) error {
	p.waitSignSlashLock.RLock()
	defer p.waitSignSlashLock.RUnlock()
	v, ok := p.waitSignSlashMsgs[sign.Address.String()]
	if !ok {
		return errors.New("slash sign request has not been verified")
	}
	_, ok = v[sign.BatchIndex]
	if !ok {
		return errors.New("slash sign request has not been verified")
	}

	return nil
}
func (p *Processor) removeWaitSlashMsg(msg tsscommon.SlashRequest) {
	p.waitSignSlashLock.Lock()
	defer p.waitSignSlashLock.Unlock()
	v, ok := p.waitSignSlashMsgs[msg.Address.String()]
	if ok {
		_, sok := v[msg.BatchIndex]
		if sok {
			delete(v, msg.BatchIndex)
		}
		if len(v) == 0 {
			delete(p.waitSignSlashMsgs, msg.Address.String())
		}
	}
}
