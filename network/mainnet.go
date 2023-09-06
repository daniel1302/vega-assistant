package network

type NetworkConfig struct {
	GenesisVersion       string
	Repository           string
	GenesisURL           string
	DataNodesRESTUrls    []string
	TendermintSeeds      []string
	BootstrapPeers       []string
	TendermintRPCServers []string
}

func MainnetConfig() NetworkConfig {
	return NetworkConfig{
		GenesisVersion: "v0.72.14",
		Repository:     "vegaprotocol/vega",
		GenesisURL:     "https://raw.githubusercontent.com/vegaprotocol/networks/master/mainnet1/genesis.json",
		DataNodesRESTUrls: []string{
			"https://api0.vega.community",
			"https://api1.vega.community",
			"https://api2.vega.community",
			"https://api3.vega.community",
		},
		TendermintSeeds: []string{
			"b0db58f5651c85385f588bd5238b42bedbe57073@13.125.55.240:26656",
			"abe207dae9367995526812d42207aeab73fd6418@18.158.4.175:26656",
			"198ecd046ebb9da0fc5a3270ee9a1aeef57a76ff@144.76.105.240:26656",
			"211e435c2162aedb6d687409d5d7f67399d198a9@65.21.60.252:26656",
			"c5b11e1d819115c4f3974d14f76269e802f3417b@34.88.191.54:26656",
			"61051c21f083ee30c835a34a0c17c5d1ceef3c62@51.178.75.45:26656",
		},
		TendermintRPCServers: []string{
			"api0.vega.community:26657",
			"api1.vega.community:26657",
			"api2.vega.community:26657",
			"api3.vega.community:26657",
		},
		BootstrapPeers: []string{
			"/dns/api1.vega.community/tcp/4001/ipfs/12D3KooWDZrusS1p2XyJDbCaWkVDCk2wJaKi6tNb4bjgSHo9yi5Q",
			"/dns/api2.vega.community/tcp/4001/ipfs/12D3KooWEH9pQd6P7RgNEpwbRyavWcwrAdiy9etivXqQZzd7Jkrh",
			"/dns/api3.vega.community/tcp/4001/ipfs/12D3KooWHSoYzEqSfUWEXfFbSnmRhWcP2WgZG2GRT8fzZzio5BTY",
		},
	}
}
