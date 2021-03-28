package main

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	iotago "github.com/iotaledger/iota.go/v2"
)

//Config
//TODO: Read this stuff from a config file and don't hardcode it
const url = "http://localhost:14265"
const threadCount = 6
const powScore = 4000
const indexationStr = "GOSPAM"
const messageStr = "This is a spam message sent via the Go spammer"
const networkIDStr = "testnet6"

var spammed int64 = 0
var errorcount int64 = 0

func main() {
	//experimental
	tr := &http.Transport{
		MaxIdleConnsPerHost: 30,
		MaxIdleConns:        50,
		IdleConnTimeout:     1 * time.Second,
		DisableCompression:  true,
	}
	client := iotago.NewNodeAPIClient(url, iotago.WithNodeAPIClientHTTPClient(&http.Client{Transport: tr}))

	for i := 0; i < threadCount; i++ {
		go createSpamThread(client, i)
	}

	//MPS calculation

	const updateMPSSeconds = 5
	//Over how many update periods the avg should be built (5s*12=60s)
	const avgPointsCount = 12
	avgPoints := [avgPointsCount]float64{}
	var updateCounter int64
	var previousSpamCount int64
	var mps float64
	var avgindex int64
	var avgmps float64
	for {
		s := atomic.LoadInt64(&spammed)
		e := atomic.LoadInt64(&errorcount)
		updateCounter++

		if updateCounter == updateMPSSeconds {
			//recalculate 5s average
			updateCounter = 0
			mps = float64(s-previousSpamCount) / float64(updateMPSSeconds)
			//recalculate 5s average
			avgPoints[avgindex] = mps
			avgindex++
			var avgMpsSum float64
			for i := 0; i < avgPointsCount; i++ {
				avgMpsSum += avgPoints[i]
			}
			avgmps = float64(avgMpsSum) / float64(avgPointsCount)
			previousSpamCount = s
		}
		if avgindex == avgPointsCount {
			avgindex = 0
		}
		//fmt.Printf("%s\r", pad)
		fmt.Printf("\rSpammed %7d   Errors %7d   Current MPS %7.2f   60s MPS %7.2f    ", s, e, mps, avgmps)
		<-time.After(time.Duration(1) * time.Second)
	}
}

func createSpamThread(client *iotago.NodeAPIClient, id int) {
	networkID := iotago.NetworkIDFromString(networkIDStr)
	index := []byte(indexationStr)
	messageBytes := []byte(messageStr)
	ctx := context.Background()
	fmt.Printf("Spammer %d started\n", id)

	for {
		//Setup the message and do POW
		msg, err := iotago.NewMessageBuilder().NetworkID(networkID).
			Payload(&iotago.Indexation{Index: index, Data: messageBytes}).
			Tips(client).ProofOfWork(ctx, powScore, 1).Build()
		if err != nil {
			atomic.AddInt64(&errorcount, 1)
			fmt.Printf("Thread %d failed message build: %s\n", id, err)
			continue
		}
		//submit the message
		err = quickSubmit(client, msg)
		if err != nil {
			atomic.AddInt64(&errorcount, 1)
			fmt.Printf("Thread %d failed message submit: %s\n", id, err)
		} else {
			atomic.AddInt64(&spammed, 1)
		}
	}
}

//This functions is equal to client.submitMessage but does not check if the node received the message
func quickSubmit(client *iotago.NodeAPIClient, m *iotago.Message) error {
	data, err := m.Serialize(iotago.DeSeriModeNoValidation)
	if err != nil {
		return err
	}

	req := &iotago.RawDataEnvelope{Data: data}
	_, err = client.Do(http.MethodPost, iotago.NodeAPIRouteMessages, req, nil)
	if err != nil {
		return err
	}
	return nil
}
