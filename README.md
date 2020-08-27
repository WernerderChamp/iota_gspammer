# Spammer

An IOTA transaction spammer.

Modes:
* 0 value spam (--type=0value).
* Static value spam without changing the ledger (--type=static)
* Conflicting spam (requires 1i; --type=conflicting)

## Installation

1. Install [Go](https://golang.org/dl/)
2. `sudo apt install build-essential`
3. Clone this repository into a folder outside of `$GOPATH`

Make sure to read the default CLI flag values below as the default values are specifically trimmed for
a private net. In order to use the spammer on mainnet, the CLI flags have to be changed.

## Run

### Standard
Execute the spammer for example with `go run -tags="pow_avx" .`,
this will start 0-value spam using the node located at `http://127.0.0.1:14265`
You can also compile to a binary using `go build -tags="pow_avx" .`

You can switch the `-tags="pow_avx"` part to the supported PoW implementations of the
[iota.go](https://github.com/iotaledger/iota.go) library, for example `pow_sse`, `pow_c` etc.

In addition to flags, you can use the config.json file to store your config.
Flags do take priority, if you are fine with always passing the arguments you can do that too
If missing, the config.json file will be created upon launch and store all flags you have passed

### Setting up conflicting spam
1. If you already used static spam, change the seed in your config.json (the faucet won't send to spent addresses)
2. Make sure the type is set to conflicting and run with --init once. This will show you the first n addresses on that seed
3. Send exactly 1i to any of these addresses and wait for confirmation. Start the spammer without --init afterwards

The spammer will create several spam bundles (defined by cycle length) which send funds between them 
If you lower this value, make sure the iota is not on an address that is no longer used now

### Maximizing Throughput
The spammer runs multiple spamming threads at the same time. The amount of instances can be changed
using the -instances command. Higher instance count can increases the TPS by better maxxing out the CPU,
but when spamming big bundles or with high MWM it can lead to lazy tips.
A good start might be the number of your physical cores, but experiment around. Running multiple instances is fine.

## Flags

* --bundlesize    Filles up all bundles to at least this size. This will round up if the value is not reachable (e.g. static spam bundle size with seclvl 2 must be a factor of 3). Defaults to 1
* --cyclelength   Specifies the number of addresses used for conflicting spam. A lower number results in more conflicts. Must be at least 2. Defaults to 3
* --init          Setups the spammer, shows spam addresses, but does not start spam
* --mwm           Specifies the minimum weight magnitude. Use 10 for comnet and 14 for mainnet. Defaults to 1
* --node          The node to use. Default is "localhost:14265"
* --seed          The seed to use for conflicting and static spam. If not provided and not set in config, a random seed will be generated.
* --type          what type of spam to spam. Can be either 0value (default), static or conflicting

To see additional flags, start the program with -h

