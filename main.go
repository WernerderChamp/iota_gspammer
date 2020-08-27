package main

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/checksum"
	"github.com/iotaledger/iota.go/consts"
	"github.com/iotaledger/iota.go/converter"
	"github.com/iotaledger/iota.go/pow"
	"github.com/iotaledger/iota.go/trinary"
	"github.com/spf13/viper"
	powsrvio "gitlab.com/powsrv.io/go/client"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var instancesNum = flag.Int("instances", 10, "spammer instance counts")
var node = flag.String("node", "http://127.0.0.1:14265", "node to use")
var powsrvioKey = flag.String("powsrvio-key", "", "the powsrv.io key to use")
var nodes = flag.String("nodes", "", "nodes to use")
var mwm = flag.Int("mwm", 1, "mwm for pow")
var tag = flag.String("tag", "SPAMMER", "tag of txs")

var addr = flag.String("addr", strings.Repeat("9", 81), "the target address of the spam")
var msg = flag.String("msg", "", "the msg to send")
var spamType = flag.String("type", "0value", "what type of spam to spam (0value, static or conflicting")
var cycleLength = flag.Int("cyclelength", 3, "length of a conflict cycle")
var bundleSize = flag.Int("bundlesize", 1, "minimum size of spam bundles. Might get rounded up for value spam")
var valueSecLvl = flag.Int("value-sec-lvl", 2, "value sec level")
var seed = flag.String("seed", strings.Repeat("9", 81), "seed to use for spam")
var doinit = flag.Bool("init", false, "if this flag is passed, the spammer setups but does not spam")

var msgTrytes, tagTrytes, seedTrytes string
var conflictBundleCount, bSize int
var addrTrytes trinary.Hash

var emptySeed = strings.Repeat("9", 81)

const configPath = "./config.json"

var config = viper.New()

func main() {
	flag.Parse()
	dryrun := *doinit
	config.BindPFlags(flag.CommandLine)
	_, err := os.Stat(configPath)
	if os.IsNotExist(err) {
		fmt.Println("Warn: config.json not found. Creating new config file.")
		//we don't want this to be true in the config
		*doinit = false
		//If no seed was provided, RNG one
		if *seed == emptySeed {
			newSeed, err := generateSeed()
			must(err)
			config.Set("seed", newSeed)
			fmt.Println("A random seed was added to the config since no seed was provided as parameter")
		}
		config.SafeWriteConfigAs(configPath)
	} else {
		config.SetConfigFile(configPath)
		err = config.ReadInConfig()
		if err != nil {
			fmt.Printf("Config could not be loaded from: %s (%s)\n", configPath, err)
		}
	}

	//cfg, _ := json.MarshalIndent(config.AllSettings(), "", "  ")
	//fmt.Printf("Settings loaded: \n %+v", string(cfg))
	msgTrytes = ""
	if len(config.GetString("msg")) > 0 {
		msgTrytes, err = converter.ASCIIToTrytes(config.GetString("msg"))
		must(err)
	}
	tagTrytes = config.GetString("tag")
	seedTrytes = trinary.Pad(config.GetString("seed"), 81)
	bSize = config.GetInt("bundlesize")

	if bSize <= 0 {
		fmt.Printf("Warn: Invalid bundle size. Assuming size 1")
		bSize = 1
	}
	conflictBundleCount = config.GetInt("cyclelength")
	if conflictBundleCount <= 1 {
		fmt.Printf("Warn: Spam cycle length must be at least 2. Value was set to 2")
		conflictBundleCount = 2
	}

	addrTrytes, err = checksum.AddChecksum(trinary.Pad(config.GetString("addr"), 81), true, consts.AddressChecksumTrytesSize)
	must(err)
	//Init bundleProvider
	iotaAPI, err := api.ComposeAPI(api.HTTPClientSettings{}) //this instance must only be used for preparing the bundles
	must(err)
	provider := bundleProvider{ready: false}

	provider.Init(config.GetString("type"), iotaAPI, consts.SecurityLevel(config.GetInt("value-sec-lvl")))

	//if we just wanted to initialize (e.g. check addresses) we can quit right here
	if dryrun {
		fmt.Println("spammer ran with --init, done")
		return
	}
	//Startup spam threads
	if len(config.GetString("nodes")) > 0 {
		split := strings.Split(config.GetString("nodes"), ",")
		for _, n := range split {
			for i := 0; i < config.GetInt("instances"); i++ {
				accSpammer(-1, provider, n)
			}
		}
	} else {
		for i := 0; i < config.GetInt("instances"); i++ {
			accSpammer(-1, provider)
		}
	}
	//TPS calculation
	pad := strings.Repeat("", 10)
	const pointsCount = 5
	points := [pointsCount]int64{}
	const avgPointsCount = 12
	avgPoints := [avgPointsCount]float64{}
	var index int
	var tps float64
	var avgindex int
	var avgtps float64
	for {
		s := atomic.LoadInt64(&spammed)
		points[index] = s
		index++
		if index == pointsCount {
			index = 0
			var deltaSum int64
			for i := 0; i < pointsCount-1; i++ {
				deltaSum += points[i+1] - points[i]
			}
			tps = float64(deltaSum) / float64(pointsCount)
			//calculating average TPS
			avgPoints[avgindex] = tps
			avgindex++
			var avgTpsSum float64
			for i := 0; i < avgPointsCount; i++ {
				avgTpsSum += avgPoints[i]
			}
			avgtps = float64(avgTpsSum) / float64(avgPointsCount)
		}
		if avgindex == avgPointsCount {
			avgindex = 0
		}
		fmt.Printf("%s\r", pad)
		fmt.Printf("\rspammed %d transactions (current tps %.2f ; 60s tps %.2f)    ", s, tps, avgtps)
		<-time.After(time.Duration(1) * time.Second)
	}
}

const seedLength = 81

var tryteAlphabetLength = byte(len(consts.TryteAlphabet))

func generateSeed() (string, error) {
	var by [seedLength]byte
	if _, err := rand.Read(by[:]); err != nil {
		return "", err
	}
	var seed string
	for _, b := range by {
		seed += string(consts.TryteAlphabet[b%tryteAlphabetLength])
	}
	return seed, nil
}

func accSpammer(stopAfter int, provider bundleProvider, nodeToUse ...string) {

	var powF pow.ProofOfWorkFunc

	if len(config.GetString("powsrvioKey")) > 0 {

		//powsrv.io config
		powClient := &powsrvio.PowClient{
			APIKey:        config.GetString("powsrvio-key"),
			ReadTimeOutMs: 5000,
			Verbose:       false,
		}

		// init powsrv.io
		err := powClient.Init()
		must(err)

		// use powsrv.io as pow func
		powF = powClient.PowFunc
	} else {
		_, powF = pow.GetFastestProofOfWorkImpl()

	}

	var n string
	if len(nodeToUse) > 0 {
		n = nodeToUse[0]
	} else {
		n = config.GetString("node")
	}

	iotaAPI, err := api.ComposeAPI(api.HTTPClientSettings{URI: n, LocalProofOfWorkFunc: powF})
	must(err)

	go func() {
		var bndl []trinary.Trytes
		for {

			tips, err := iotaAPI.GetTransactionsToApprove(uint64(config.GetInt("depth")))
			if err != nil {
				fmt.Printf("error sending: %s\n", err.Error())
				continue
			}
			bndl = provider.getNextBundle()
			powedBndl, err := iotaAPI.AttachToTangle(tips.TrunkTransaction, tips.BranchTransaction, uint64(config.GetInt("mwm")), bndl)
			if err != nil {
				fmt.Printf("error doing PoW: %s\n", err.Error())
				continue
			}

			_, err = iotaAPI.BroadcastTransactions(powedBndl...)
			if err != nil {
				fmt.Printf("error sending: %s\n", err.Error())
				continue
			}
			if stopAfter != -1 {
				stopAfter--
				if stopAfter == 0 {
					break
				}
			} else {
				atomic.AddInt64(&spammed, int64(len(powedBndl)))
			}
		}
	}()
}

var spammed int64 = 0
