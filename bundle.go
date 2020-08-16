package main

import (
	"time"

	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/bundle"
	"github.com/iotaledger/iota.go/consts"
	. "github.com/iotaledger/iota.go/guards/validators"
	"github.com/iotaledger/iota.go/signing"
	"github.com/iotaledger/iota.go/transaction"
	"github.com/iotaledger/iota.go/trinary"
)

func PrepareTransfers(apii *api.API, seed trinary.Trytes, transfers bundle.Transfers, opts api.PrepareTransfersOptions) ([]trinary.Trytes, error) {
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
