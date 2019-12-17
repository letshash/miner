package miner

import (
	"encoding/hex"
	"fmt"
	"github.com/hacash/core/interfaces"
	"github.com/hacash/mint"
	"github.com/hacash/mint/coinbase"
	"github.com/hacash/mint/difficulty"
	"sync/atomic"
	"time"
)

func (m *Miner) doStartMining() {

	defer func() {
		// set mining status stop
		atomic.StoreUint32(m.isMiningStatus, 0)
	}()

	// start mining
	last, err := m.blockchain.State().ReadLastestBlockHeadAndMeta()
	if err != nil {
		panic(err)
	}
	// pick up txs from pool
	pikuptxs := m.txpool.CopyTxsOrderByFeePurity(last.GetHeight()+1, 2000, mint.SingleBlockMaxSize)
	// create next block
	nextblock, totaltxsize, e1 := m.blockchain.CreateNextBlockByValidateTxs(pikuptxs)
	if e1 != nil {
		panic(e1)
	}
	nextblockHeight := nextblock.GetHeight()

	if nextblockHeight%mint.AdjustTargetDifficultyNumberOfBlocks == 0 {
		diff1 := last.GetDifficulty()
		diff2 := nextblock.GetDifficulty()
		tarhx1 := hex.EncodeToString(difficulty.Uint32ToHash(last.GetHeight(), diff1))
		tarhx2 := hex.EncodeToString(difficulty.Uint32ToHash(nextblockHeight, diff2))
		costtime, err := m.blockchain.ReadPrev288BlockTimestamp(nextblockHeight)
		if err == nil {
			costtime = nextblock.GetTimestamp() - costtime
		}
		targettime := mint.AdjustTargetDifficultyNumberOfBlocks * mint.EachBlockRequiredTargetTime
		fmt.Printf("== %d == -> == (%ds/%ds) == target difficulty change: %d -> %d , %s -> %s \n",
			nextblockHeight,
			costtime, targettime,
			diff1, diff2,
			string([]byte(tarhx1)[0:26]),
			string([]byte(tarhx2)[0:26]),
		)
	}

	fmt.Printf("do mining... block height: %d, txs: %d, size: %fkb, prev hash: %s..., difficulty: %d, time: %s\n",
		nextblockHeight,
		nextblock.GetTransactionCount()-1,
		float64(totaltxsize)/1024,
		string([]byte(nextblock.GetPrevHash().ToHex())[0:20]),
		nextblock.GetDifficulty(),
		time.Unix(int64(nextblock.GetTimestamp()), 0).Format("01/02 15:04:05"),
	)

	// excavate block
	backBlockCh := make(chan interfaces.Block, 1)
	m.powmaster.Excavate(nextblock, backBlockCh)

	//fmt.Println("finifsh m.powmaster.Excavate nextblock")

	var miningSuccessBlock interfaces.Block = nil
	select {
	case miningSuccessBlock = <-backBlockCh:
	case <-m.stopSignCh:
		// fmt.Println("return <- m.stopSignCh:")
		return // stop mining
	}
	// mark stop
	atomic.StoreUint32(m.isMiningStatus, 0)

	//fmt.Println("select miningSuccessBlock ok", miningSuccessBlock)

	// mining success
	if miningSuccessBlock != nil {
		miningSuccessBlock.SetOriginMark("mining")
		inserterr := m.blockchain.InsertBlock(miningSuccessBlock)
		if inserterr == nil {
			coinbaseStr := ""
			coinbasetx := miningSuccessBlock.GetTransactions()[0]
			coinbaseStr += coinbasetx.GetAddress().ToReadable()
			coinbaseStr += " + " + coinbase.BlockCoinBaseReward(miningSuccessBlock.GetHeight()).ToFinString()
			// show success
			fmt.Printf("⬤ mining new block successfully! height: %d, txs: %d, hash: %s, prev hash: %s..., coinbase: %s\n",
				miningSuccessBlock.GetHeight(),
				miningSuccessBlock.GetTransactionCount()-1,
				miningSuccessBlock.Hash().ToHex(),
				string([]byte(miningSuccessBlock.GetPrevHash().ToHex())[0:20]),
				coinbaseStr,
			)
		} else {
			fmt.Println("[Miner Error]", inserterr.Error())
			m.StartMining()
		}
	}
}