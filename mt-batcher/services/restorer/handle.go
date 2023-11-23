package restorer

import (
	gecho "github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/tenderly/optimism/l2geth/core/types"
	"github.com/tenderly/optimism/l2geth/rollup/eigenda"
)

const (
	maxCallReceiveMessageSize = 314572800
)

type RollupStoreRequest struct {
	BatchIndex int64 `json:"batch_index"`
}

type TransactionRequest struct {
	StoreNumber uint32 `json:"store_number"`
}

type DataStoreRequest struct {
	FromStoreNumber   string `json:"from_store_number"`
	EigenContractAddr string `json:"eigen_contract_addr"`
}

type DataStoreIdRequest struct {
	StoreId string `json:"store_id"`
}

type TransactionListResponse struct {
	BlockNumber string `json:"BlockNumber"`
	TxHash      string `json:"TxHash"`
}

type TransactionInfoListResponse struct {
	BlockNumber string                `json:"BlockNumber"`
	TxHash      string                `json:"TxHash"`
	TxMeta      types.TransactionMeta `json:"TxMeta"`
	TxData      types.Transaction     `json:"TxDetail"`
}

func (s *DaService) GetLatestTransactionBatchIndex(c gecho.Context) error {
	batchIndex, err := s.Cfg.EigenContract.RollupBatchIndex(&bind.CallOpts{})
	if err != nil {
		return c.JSON(http.StatusBadRequest, errors.New("fail to get batch index"))
	}
	return c.JSON(http.StatusOK, batchIndex.Uint64())
}

func (s *DaService) GetRollupStoreByRollupBatchIndex(c gecho.Context) error {
	var rsReq RollupStoreRequest
	if err := c.Bind(&rsReq); err != nil {
		return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	}
	rollupStore, err := s.Cfg.EigenContract.GetRollupStoreByRollupBatchIndex(&bind.CallOpts{}, big.NewInt(rsReq.BatchIndex))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errors.New("get rollup store fail"))
	}
	rsRep := &eigenda.RollupStoreResponse{
		OriginDataStoreId: rollupStore.OriginDataStoreId,
		DataStoreId:       rollupStore.DataStoreId,
		ConfirmAt:         rollupStore.ConfirmAt,
		Status:            rollupStore.Status,
	}
	return c.JSON(http.StatusOK, rsRep)
}

func (s *DaService) GetBatchTransactionByDataStoreId(c gecho.Context) error {
	//var txReq TransactionRequest
	//if err := c.Bind(&txReq); err != nil {
	//	log.Error("invalid request params", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	//}
	//conn, err := grpc.Dial(s.Cfg.RetrieverSocket, grpc.WithInsecure())
	//if err != nil {
	//	log.Error("disperser Cannot connect to", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("disperser Cannot connect to"))
	//}
	//defer conn.Close()
	//client := pb.NewDataRetrievalClient(conn)
	//
	//opt := grpc.MaxCallRecvMsgSize(maxCallReceiveMessageSize)
	//request := &pb.FramesAndDataRequest{
	//	DataStoreId: txReq.StoreNumber,
	//}
	//reply, err := client.RetrieveFramesAndData(s.Ctx, request, opt)
	//if err != nil {
	//	log.Error("retrieve frames and data error", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("recovery data fail"))
	//}
	//if len(reply.GetData()) >= 31*s.Cfg.EigenLayerNode {
	//	return c.JSON(http.StatusOK, reply.GetData())
	//} else {
	//	return c.JSON(http.StatusBadRequest, errors.New("retrieve data is empty, please check da date"))
	//}
	return nil
}

func (s *DaService) GetDtlBatchTransactionByDataStoreId(c gecho.Context) error {
	//var txReq TransactionRequest
	//if err := c.Bind(&txReq); err != nil {
	//	log.Error("invalid request params", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	//}
	//conn, err := grpc.Dial(s.Cfg.RetrieverSocket, grpc.WithInsecure())
	//if err != nil {
	//	log.Error("disperser Cannot connect to", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("disperser Cannot connect to"))
	//}
	//defer conn.Close()
	//client := pb.NewDataRetrievalClient(conn)
	//
	//opt := grpc.MaxCallRecvMsgSize(maxCallReceiveMessageSize)
	//request := &pb.FramesAndDataRequest{
	//	DataStoreId: txReq.StoreNumber,
	//}
	//reply, err := client.RetrieveFramesAndData(s.Ctx, request, opt)
	//if err != nil {
	//	log.Error("retrieve frames and data error", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("recovery data fail"))
	//}
	//if len(reply.GetData()) >= 31*s.Cfg.EigenLayerNode {
	//	data := reply.GetData()
	//
	//	batchTxn := new([]eigenda.BatchTx)
	//	batchRlpStream := rlp.NewStream(bytes.NewBuffer(data), uint64(len(data)))
	//	err = batchRlpStream.Decode(batchTxn)
	//	if err != nil {
	//		return c.JSON(http.StatusBadRequest, errors.New("decode data fail"))
	//	}
	//	var TxnRep []*TransactionInfoListResponse
	//	newBatchTxn := *batchTxn
	//	for i := 0; i < len(newBatchTxn); i++ {
	//		l2Tx := new(types.Transaction)
	//		txDecodeMetaData := new(eigenda.TransactionMeta)
	//		if newBatchTxn[i].TxMeta == nil {
	//			log.Error("Batch tx metadata shouldn't be nil")
	//			return c.JSON(http.StatusBadRequest, errors.New("Batch tx metadata shouldn't be nil"))
	//		}
	//		err = json.Unmarshal(newBatchTxn[i].TxMeta, txDecodeMetaData)
	//		if err != nil {
	//			return c.JSON(http.StatusBadRequest, errors.New("Unmarshal json fail"))
	//		}
	//		rlpStream := l2rlp.NewStream(bytes.NewBuffer(newBatchTxn[i].RawTx), 0)
	//		if err := l2Tx.DecodeRLP(rlpStream); err != nil {
	//			return c.JSON(http.StatusBadRequest, errors.New("Decode RLP fail"))
	//		}
	//		newBlockNumber := new(big.Int).SetBytes(newBatchTxn[i].BlockNumber)
	//
	//		var queueOrigin types.QueueOrigin
	//		var l1MessageSender *common2.Address
	//		if txDecodeMetaData.QueueIndex == nil {
	//			queueOrigin = types.QueueOriginSequencer
	//			l1MessageSender = nil
	//		} else {
	//			queueOrigin = types.QueueOriginL1ToL2
	//			addrLs := common2.HexToAddress("")
	//			l1MessageSender = &addrLs
	//		}
	//		realTxMeta := &types.TransactionMeta{
	//			L1BlockNumber:   txDecodeMetaData.L1BlockNumber,
	//			L1Timestamp:     txDecodeMetaData.L1Timestamp,
	//			L1MessageSender: l1MessageSender,
	//			QueueOrigin:     queueOrigin,
	//			Index:           txDecodeMetaData.Index,
	//			QueueIndex:      txDecodeMetaData.QueueIndex,
	//			RawTransaction:  txDecodeMetaData.RawTransaction,
	//		}
	//		txSl := &TransactionInfoListResponse{
	//			BlockNumber: newBlockNumber.String(),
	//			TxHash:      l2Tx.Hash().String(),
	//			TxMeta:      *realTxMeta,
	//			TxData:      *l2Tx,
	//		}
	//		TxnRep = append(TxnRep, txSl)
	//	}
	//	return c.JSON(http.StatusOK, TxnRep)
	//} else {
	//	return c.JSON(http.StatusBadRequest, errors.New("retrieve data is empty, please check da date"))
	//}
	return nil
}

func (s *DaService) GetDataStoreList(c gecho.Context) error {
	//var dsReq DataStoreRequest
	//if err := c.Bind(&dsReq); err != nil {
	//	log.Error("invalid request params", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	//}
	//var query struct {
	//	DataStores []graphView.DataStoreGql `graphql:"dataStores(where:{storeNumber_gt: $lastStoreNumber,confirmer: $confirmer,confirmed:true})"`
	//}
	//variables := map[string]interface{}{
	//	"lastStoreNumber": graphql.String(dsReq.FromStoreNumber),
	//	"confirmer":       graphql.String(strings.ToLower(dsReq.EigenContractAddr)),
	//}
	//err := s.GraphqlClient.Query(context.Background(), &query, variables)
	//if err != nil {
	//	log.Error("GetExpiringDataStores error")
	//	return c.JSON(http.StatusBadRequest, errors.New("iGetExpiringDataStores error"))
	//}
	//if len(query.DataStores) == 0 {
	//	return c.JSON(http.StatusBadRequest, errors.New("no new stores"))
	//}
	//return c.JSON(http.StatusOK, query.DataStores)
	return nil
}

func (s *DaService) getDataStoreById(c gecho.Context) error {
	//var dsIdReq DataStoreIdRequest
	//if err := c.Bind(&dsIdReq); err != nil {
	//	log.Error("invalid request params", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	//}
	//var query struct {
	//	DataStore graphView.DataStoreGql `graphql:"dataStore(id: $storeId)"`
	//}
	//variables := map[string]interface{}{
	//	"storeId": graphql.String(dsIdReq.StoreId),
	//}
	//err := s.GraphqlClient.Query(context.Background(), &query, variables)
	//if err != nil {
	//	log.Error("query data from graphql fail", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("query data from graphql fail"))
	//}
	//return c.JSON(http.StatusOK, query.DataStore)
	return nil
}

func (s *DaService) GetTransactionListByStoreNumber(c gecho.Context) error {
	//var txReq TransactionRequest
	//if err := c.Bind(&txReq); err != nil {
	//	log.Error("invalid request params", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("invalid request params"))
	//}
	//conn, err := grpc.Dial(s.Cfg.RetrieverSocket, grpc.WithInsecure())
	//if err != nil {
	//	return c.JSON(http.StatusBadRequest, errors.New("disperser Cannot connect to"))
	//}
	//defer conn.Close()
	//client := pb.NewDataRetrievalClient(conn)
	//
	//opt := grpc.MaxCallRecvMsgSize(1024 * 1024 * 300)
	//request := &pb.FramesAndDataRequest{
	//	DataStoreId: txReq.StoreNumber,
	//}
	//reply, err := client.RetrieveFramesAndData(s.Ctx, request, opt)
	//if err != nil {
	//	log.Error("RetrieveFramesAndData retrieve frames and data error", "err", err)
	//	return c.JSON(http.StatusBadRequest, errors.New("RetrieveFramesAndData error"))
	//}
	//data := reply.GetData()
	//if len(data) >= 31*s.Cfg.EigenLayerNode {
	//	batchTxn := new([]eigenda.BatchTx)
	//	batchRlpStream := rlp.NewStream(bytes.NewBuffer(data), uint64(len(data)))
	//	err = batchRlpStream.Decode(batchTxn)
	//	if err != nil {
	//		return c.JSON(http.StatusBadRequest, errors.New("decode data fail"))
	//	}
	//	var TxnRep []*TransactionListResponse
	//	newBatchTxn := *batchTxn
	//	for i := 0; i < len(newBatchTxn); i++ {
	//		l2Tx := new(types.Transaction)
	//		rlpStream := l2rlp.NewStream(bytes.NewBuffer(newBatchTxn[i].RawTx), 0)
	//		if err := l2Tx.DecodeRLP(rlpStream); err != nil {
	//			log.Error("Decode RLP fail")
	//			return c.JSON(http.StatusBadRequest, errors.New("Decode RLP fail"))
	//		}
	//		newBlockNumber := new(big.Int).SetBytes(newBatchTxn[i].BlockNumber)
	//		txSl := &TransactionListResponse{
	//			BlockNumber: newBlockNumber.String(),
	//			TxHash:      l2Tx.Hash().String(),
	//		}
	//		TxnRep = append(TxnRep, txSl)
	//	}
	//	return c.JSON(http.StatusOK, TxnRep)
	//} else {
	//	return c.JSON(http.StatusBadRequest, errors.New("retrieve data is empty, please check da data batch"))
	//}
	return nil
}
