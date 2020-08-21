package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/iotaledger/iota.go/address"
	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/bundle"
	"github.com/iotaledger/iota.go/consts"
	. "github.com/iotaledger/iota.go/guards/validators"
	"github.com/iotaledger/iota.go/signing"
	"github.com/iotaledger/iota.go/transaction"
	"github.com/iotaledger/iota.go/trinary"
)

type bundleProvider struct {
	isValue       bool
	attachBundles [][]string
	ready         bool
}

func (b *bundleProvider) getNextBundle() []string {
	if !b.ready {
		panic("Asked for next bundle before ready")
	}
	return b.attachBundles[rand.Intn(len(b.attachBundles))]
}

func (b *bundleProvider) Init(spamType string, apiLocal *api.API, valueSecLvl consts.SecurityLevel) {

	switch spamType {
	case "static":
		b.initStaticSpam(apiLocal, valueSecLvl)
	case "0value":
		b.init0ValueSpam(apiLocal)
	case "conflicting":
		b.initSimpleConflictingSpam(apiLocal, valueSecLvl)
	default:
		fmt.Println("Warn: Invalid Spam type. Spamming 0value")
		b.init0ValueSpam(apiLocal)
	}
	b.ready = true //we are now ready to query
}

func (b *bundleProvider) init0ValueSpam(apiLocal *api.API) {
	var spamTransfer = []bundle.Transfer{{Address: targetAddr, Tag: *tag}}
	if len(*msg) > 0 {
		spamTransfer[0].Message = *msgTrytes
	}
	var bndl, err = apiLocal.PrepareTransfers(emptySeed, spamTransfer, api.PrepareTransfersOptions{})
	if err != nil {
		fmt.Printf("error preparing transfer: %s\n", err.Error())
		panic(err)
	}
	b.attachBundles = make([][]string, 1)
	b.attachBundles[0] = bndl
}

func (b *bundleProvider) initStaticSpam(apiLocal *api.API, valueSecLvl consts.SecurityLevel) {
	trnsf := []bundle.Transfer{}
	inputs := []api.Input{}
	//add transfers until the minimum size is exceeded
	spendcount := int(math.Ceil(float64(*bundleSize) / (float64(valueSecLvl) + 1)))
	for i := 0; i < spendcount; i++ {
		addr, err := address.GenerateAddress(*seed, uint64(i), consts.SecurityLevel(valueSecLvl), true)
		if err != nil {
			fmt.Printf("error creating address: %s\n", err.Error())
			panic(err)
		}
		trnsf = append(trnsf, bundle.Transfer{
			Address: addr,
			Tag:     *tag,
			Value:   142650000,
			Message: *msg,
		})
		inputs = append(inputs, api.Input{
			Address:  addr,
			KeyIndex: uint64(i),
			Security: consts.SecurityLevel(valueSecLvl),
			Balance:  142650000,
		})
	}
	var bndl []string
	var err error
	bndl, err = prepareTransfers(apiLocal, *seed, trnsf, api.PrepareTransfersOptions{Inputs: inputs})
	if err != nil {
		fmt.Printf("error preparing transfer: %s\n", err.Error())
		panic(err)
	}
	b.attachBundles = make([][]string, 1)
	b.attachBundles[0] = bndl
}

func (b *bundleProvider) initSimpleConflictingSpam(apiLocal *api.API, valueSecLvl consts.SecurityLevel) {
	addresses := make([]string, *cycleLength)
	for i := 0; i < *cycleLength; i++ {
		addr, err := address.GenerateAddress(*seed, uint64(i), consts.SecurityLevel(valueSecLvl), true)
		if err != nil {
			fmt.Printf("error creating address: %s\n", err.Error())
			panic(err)
		}
		addresses[i] = addr
	}
	fmt.Println("First address on seed: ", addresses[0])
	var bndl []string
	var err error
	b.attachBundles = make([][]string, *cycleLength)
	for i := 0; i < *cycleLength; i++ {
		trnsf := []bundle.Transfer{}
		inputs := []api.Input{}
		//send iota to the next address, last one sends to first one
		if i == *cycleLength-1 {
			trnsf = append(trnsf, bundle.Transfer{
				Address: addresses[0],
				Tag:     *tag,
				Value:   1,
				Message: *msg,
			})
		} else {
			trnsf = append(trnsf, bundle.Transfer{
				Address: addresses[i+1],
				Tag:     *tag,
				Message: *msg,
				Value:   1,
			})
		}
		inputs = append(inputs, api.Input{
			Address:  addresses[i],
			KeyIndex: uint64(i),
			Security: consts.SecurityLevel(valueSecLvl),
			Balance:  1,
		})
		//pad bundle, so it has the minimum size
		for j := int(valueSecLvl) + 1; j < *bundleSize; j++ {
			trnsf = append(trnsf, bundle.Transfer{
				Address: targetAddr,
				Tag:     *tag,
				Value:   0,
			})
		}
		bndl, err = prepareTransfers(apiLocal, *seed, trnsf, api.PrepareTransfersOptions{Inputs: inputs})
		if err != nil {
			fmt.Printf("error preparing transfer: %s\n", err.Error())
			panic(err)
		}
		b.attachBundles[i] = bndl
	}
}

func prepareTransfers(api *api.API, seed trinary.Trytes, transfers bundle.Transfers, opts api.PrepareTransfersOptions) ([]trinary.Trytes, error) {
	opts = getPrepareTransfersDefaultOptions(opts)

	if err := Validate(ValidateSeed(seed), ValidateSecurityLevel(opts.Security)); err != nil {
		return nil, err
	}

	for i := range transfers {
		if err := Validate(ValidateAddresses(transfers[i].Value != 0, transfers[i].Address)); err != nil {
			return nil, err
		}
	}

	var timestamp uint64
	txs := transaction.Transactions{}

	if opts.Timestamp != nil {
		timestamp = *opts.Timestamp
	} else {
		timestamp = uint64(time.Now().UnixNano() / int64(time.Second))
	}

	var totalOutput uint64
	for i := range transfers {
		totalOutput += transfers[i].Value
	}

	// add transfers
	outEntries, err := bundle.TransfersToBundleEntries(timestamp, transfers...)
	if err != nil {
		return nil, err
	}
	for i := range outEntries {
		txs = bundle.AddEntry(txs, outEntries[i])
	}

	// add input transactions
	var totalInput uint64
	for i := range opts.Inputs {
		if err := Validate(ValidateAddresses(opts.Inputs[i].Balance != 0, opts.Inputs[i].Address)); err != nil {
			return nil, err
		}
		totalInput += opts.Inputs[i].Balance
		input := &opts.Inputs[i]
		bndlEntry := bundle.BundleEntry{
			Address:   input.Address[:consts.HashTrytesSize],
			Value:     -int64(input.Balance),
			Length:    uint64(input.Security),
			Timestamp: timestamp,
		}
		txs = bundle.AddEntry(txs, bndlEntry)
	}

	// verify whether provided inputs fulfill threshold value
	if totalInput < totalOutput {
		return nil, consts.ErrInsufficientBalance
	}

	// finalize bundle by adding the bundle hash
	finalizedBundle, err := bundle.Finalize(txs)
	if err != nil {
		return nil, err
	}

	// compute signatures for all input txs
	normalizedBundleHash := signing.NormalizedBundleHash(finalizedBundle[0].Bundle)

	signedFrags := []trinary.Trytes{}
	for i := range opts.Inputs {
		input := &opts.Inputs[i]
		subseed, err := signing.Subseed(seed, input.KeyIndex)
		if err != nil {
			return nil, err
		}
		var sec consts.SecurityLevel
		if input.Security == 0 {
			sec = consts.SecurityLevelMedium
		} else {
			sec = input.Security
		}

		prvKey, err := signing.Key(subseed, sec)
		if err != nil {
			return nil, err
		}

		frags := make([]trinary.Trytes, input.Security)
		for i := 0; i < int(input.Security); i++ {
			signedFragTrits, err := signing.SignatureFragment(
				normalizedBundleHash[i*consts.HashTrytesSize/3:(i+1)*consts.HashTrytesSize/3],
				prvKey[i*consts.KeyFragmentLength:(i+1)*consts.KeyFragmentLength],
			)
			if err != nil {
				return nil, err
			}
			frags[i] = trinary.MustTritsToTrytes(signedFragTrits)
		}

		signedFrags = append(signedFrags, frags...)
	}

	// add signed fragments to txs
	var indexFirstInputTx int
	for i := range txs {
		if txs[i].Value < 0 {
			indexFirstInputTx = i
			break
		}
	}

	txs = bundle.AddTrytes(txs, signedFrags, indexFirstInputTx)

	// finally return built up txs as raw trytes
	return transaction.MustFinalTransactionTrytes(txs), nil
}

func getPrepareTransfersDefaultOptions(options api.PrepareTransfersOptions) api.PrepareTransfersOptions {
	if options.Security == 0 {
		options.Security = consts.SecurityLevelMedium
	}
	if options.Inputs == nil {
		options.Inputs = []api.Input{}
	}
	return options
}
