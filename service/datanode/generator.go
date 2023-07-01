package datanode

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/daniel1302/vega-asistant/github"
	"github.com/daniel1302/vega-asistant/network"
	"github.com/daniel1302/vega-asistant/utils"
	"github.com/daniel1302/vega-asistant/vegacmd"
)

type DataNodeGenerator struct {
	userSettings  GenerateSettings
	networkConfig network.NetworkConfig
}

func NewDataNodeGenerator(
	settings GenerateSettings,
	networkConfig network.NetworkConfig,
) (*DataNodeGenerator, error) {
	return &DataNodeGenerator{
		userSettings:  settings,
		networkConfig: networkConfig,
	}, nil
}

func (gen *DataNodeGenerator) Run(logger *zap.SugaredLogger) error {
	outputDir, err := os.MkdirTemp("", "vega-assistant")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	//	defer os.RemoveAll(outputDir)

	logger.Info("Downloading vega binary")
	vegaBinaryPath, err := github.DownloadArtifact(
		gen.networkConfig.Repository,
		gen.userSettings.MainnetVersion,
		outputDir,
		github.ArtifactVega,
	)
	if err != nil {
		return fmt.Errorf("failed to download vega binary: %w", err)
	}
	logger.Infof("Vega downloaded to %s", vegaBinaryPath)

	logger.Info("Downloading visor binary")
	visorBinaryPath, err := github.DownloadArtifact(
		gen.networkConfig.Repository,
		gen.userSettings.MainnetVersion,
		outputDir,
		github.ArtifactVisor,
	)
	if err != nil {
		return fmt.Errorf("failed to download visor binary: %w", err)
	}
	logger.Infof("Visor downloaded to %s", visorBinaryPath)

	logger.Info("Checking binaries versions")
	vegaVersion, err := utils.ExecuteBinary(vegaBinaryPath, []string{"version"}, nil)
	if err != nil {
		return fmt.Errorf("failed to check vega version: %w", err)
	}
	logger.Infof("Vega version is %s", vegaVersion)
	visorVersion, err := utils.ExecuteBinary(visorBinaryPath, []string{"version"}, nil)
	if err != nil {
		return fmt.Errorf("failed to check visor version: %w", err)
	}
	logger.Infof("Visor version is %s", visorVersion)

	if err := gen.initNode(logger, visorBinaryPath, vegaBinaryPath); err != nil {
		return fmt.Errorf("failed to init vega node: %w", err)
	}

	if err := gen.prepareVisorHome(logger); err != nil {
		return fmt.Errorf("failed to prepare visor home: %w", err)
	}

	if err := gen.copyBinaries(logger, vegaBinaryPath, visorBinaryPath); err != nil {
		return fmt.Errorf("failed to copy binaries to visor home: %w", err)
	}

	if err := gen.updateConfigs(logger); err != nil {
		return fmt.Errorf("failed to update config files for the node: %w", err)
	}

	if err := gen.downloadGenesis(logger); err != nil {
		return fmt.Errorf("failed to download genesis: %w", err)
	}
	return nil
}

func (gen *DataNodeGenerator) downloadGenesis(logger *zap.SugaredLogger) error {
	genesisDestination := filepath.Join(gen.userSettings.TendermintHome, vegacmd.GenesisPath)
	logger.Infof("Downloading genesis.json file from %s", gen.networkConfig.GenesisURL)
	if err := utils.DownloadFile(gen.networkConfig.GenesisURL, genesisDestination); err != nil {
		return fmt.Errorf("failed to download genesis: %w", err)
	}
	logger.Infof("Genesis downloaded to %s", genesisDestination)

	return nil
}

func (gen *DataNodeGenerator) copyBinaries(
	logger *zap.SugaredLogger,
	vegaBinaryPath, visorBinaryPath string,
) error {
	vegavisorDstFilePath := filepath.Join(gen.userSettings.VisorHome, "visor")
	logger.Infof("Copying vegavisor from %s to %s", visorBinaryPath, vegavisorDstFilePath)
	if err := utils.CopyFile(visorBinaryPath, vegavisorDstFilePath); err != nil {
		return fmt.Errorf("failed to copy visor binary: %w", err)
	}
	logger.Info("Visor binary copied")

	version := gen.userSettings.MainnetVersion
	if gen.userSettings.Mode == StartFromBlock0 {
		version = "genesis"
	}

	vegaDstFilePath := filepath.Join(gen.userSettings.VisorHome, version, "vega")
	logger.Infof("Copying vega from %s to %s", vegaBinaryPath, vegaDstFilePath)
	if err := utils.CopyFile(vegaBinaryPath, vegaDstFilePath); err != nil {
		return fmt.Errorf("failed to copy vega binary: %w", err)
	}
	logger.Info("Vega binary copied")

	versionDirectory := filepath.Join(gen.userSettings.VisorHome, version)
	currentDirectory := filepath.Join(gen.userSettings.VisorHome, "current")
	logger.Infof("Creating symlink from %s to %s", versionDirectory, currentDirectory)
	if err := os.Symlink(versionDirectory, currentDirectory); err != nil {
		return fmt.Errorf(
			"failed to create symlink from %s to %s: %w",
			versionDirectory,
			currentDirectory,
			err,
		)
	}
	logger.Info("Symlink created")

	return nil
}

func (gen *DataNodeGenerator) prepareVisorHome(logger *zap.SugaredLogger) error {
	runConfigDirPath := filepath.Join(gen.userSettings.VisorHome, gen.userSettings.MainnetVersion)
	version := gen.userSettings.MainnetVersion

	if gen.userSettings.Mode == StartFromBlock0 {
		runConfigDirPath = filepath.Join(gen.userSettings.VisorHome, "genesis")
		version = "genesis"
	}

	logger.Infof("Preparing %s folder for vega", runConfigDirPath)
	if err := os.MkdirAll(runConfigDirPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to make directory: %w", err)
	}
	logger.Infof("Folder %s created", runConfigDirPath)

	runConfigPath := filepath.Join(runConfigDirPath, "run-config.toml")
	logger.Infof("Preparing run-config toml file in %s", runConfigPath)
	runConfigContent, err := vegacmd.TemplateVisorRunConfig(
		version,
		gen.userSettings.VegaHome,
		gen.userSettings.TendermintHome,
	)
	if err != nil {
		return fmt.Errorf("failed to generate run-config.toml from template: %w", err)
	}
	if err := os.WriteFile(runConfigPath, []byte(runConfigContent), os.ModePerm); err != nil {
		return fmt.Errorf("failed to write run-config.toml in %s: %w", runConfigContent, err)
	}
	logger.Infof("The run-config.toml file saved in %s", runConfigPath)

	return nil
}

func (gen *DataNodeGenerator) updateConfigs(logger *zap.SugaredLogger) error {
	dataNodeConfig := map[string]interface{}{
		"SQLStore.ConnectionConfig.Host":      gen.userSettings.SQLCredentials.Host,
		"SQLStore.ConnectionConfig.Port":      gen.userSettings.SQLCredentials.Port,
		"SQLStore.ConnectionConfig.Username":  gen.userSettings.SQLCredentials.User,
		"SQLStore.ConnectionConfig.Password":  gen.userSettings.SQLCredentials.Pass,
		"SQLStore.ConnectionConfig.Database":  gen.userSettings.SQLCredentials.DatabaseName,
		"SQLStore.WipeOnStartup":              true,
		"NetworkHistory.Store.BootstrapPeers": gen.networkConfig.BootstrapPeers,
		"NetworkHistory.Initialise.Timeout":   "4h",
		"API.RateLimit.Rate":                  300.0,
		"API.RateLimit.Burst":                 1000,
	}

	vegaConfig := map[string]interface{}{
		"Snapshot.StartHeight":      -1,
		"Broker.Socket.Enabled":     true,
		"Broker.Socket.DialTimeout": "4h",
	}

	tendermintConfig := map[string]interface{}{
		"p2p.seeds":              strings.Join(gen.networkConfig.TendermintSeeds, ","),
		"pex":                    true,
		"statesync.enable":       false,
		"statesync.rpc_servers":  strings.Join(gen.networkConfig.TendermintRPCServers, ","),
		"statesync.trust_period": "672h0m0s",
	}

	vegavisorConfig := map[string]interface{}{
		"maxNumberOfFirstConnectionRetries": 43200,
		"autoInstall.enabled":               true,
		"autoInstall.repositoryOwner":       strings.Split(gen.networkConfig.Repository, "/")[0],
		"autoInstall.repository":            strings.Split(gen.networkConfig.Repository, "/")[1],
		"autoInstall.asset.name": fmt.Sprintf(
			"vega-%s-%s.zip",
			runtime.GOOS,
			runtime.GOARCH,
		),
		"autoInstall.asset.binaryName": "vega",
	}

	if gen.userSettings.Mode == StartFromNetworkHistory {
		if gen.userSettings.LatestSnapshot.BlockHash == "" {
			return fmt.Errorf(
				"cannot start vega from the network-history when latest snapshot is empty",
			)
		}

		trustHeight, err := strconv.Atoi(gen.userSettings.LatestSnapshot.BlockHeight)
		if err != nil {
			return fmt.Errorf("failed to convert trust block height from string to int: %w", err)
		}

		dataNodeConfig["AutoInitialiseFromNetworkHistory"] = true
		tendermintConfig["statesync.enable"] = true
		tendermintConfig["statesync.trust_height"] = trustHeight
		tendermintConfig["statesync.trust_hash"] = gen.userSettings.LatestSnapshot.BlockHash
	}

	dataNodeConfigPath := filepath.Join(gen.userSettings.DataNodeHome, vegacmd.DataNodeConfigPath)
	logger.Infof(
		"Updating data-node config(%s). New parameters: %v",
		&dataNodeConfigPath,
		dataNodeConfig,
	)
	if err := utils.UpdateConfig(dataNodeConfigPath, "toml", dataNodeConfig); err != nil {
		return fmt.Errorf("failed to update the data-node config; %w", err)
	}
	logger.Info("Data-node config updated")

	vegaConfigPath := filepath.Join(gen.userSettings.VegaHome, vegacmd.CoreConfigPath)
	logger.Infof("Updating vega-core config(%s). New parameters: %v", vegaConfigPath, vegaConfig)
	if err := utils.UpdateConfig(vegaConfigPath, "toml", vegaConfig); err != nil {
		return fmt.Errorf("failed to update the vega config; %w", err)
	}
	logger.Info("Vega-core config updated")

	tendermintConfigPath := filepath.Join(
		gen.userSettings.TendermintHome,
		vegacmd.TenderminConfigPath,
	)
	logger.Infof(
		"Updating tendermint config(%s). New parameters: %v",
		tendermintConfigPath,
		tendermintConfig,
	)
	if err := utils.UpdateConfig(tendermintConfigPath, "toml", tendermintConfig); err != nil {
		return fmt.Errorf("failed to update the tendermint config; %w", err)
	}
	logger.Info("Tendermint config updated")

	vegavisorConfigPath := filepath.Join(gen.userSettings.VisorHome, vegacmd.VegavisorConfigPath)
	logger.Infof(
		"Updating vegavisor config(%s). New parameters: %v",
		vegavisorConfigPath,
		vegavisorConfig,
	)
	if err := utils.UpdateConfig(vegavisorConfigPath, "toml", vegavisorConfig); err != nil {
		return fmt.Errorf("failed to update vegavisor config: %w", err)
	}
	logger.Info("Vegavisor config updated")

	return nil
}

func (gen *DataNodeGenerator) initNode(
	logger *zap.SugaredLogger,
	visorBinary, vegaBinary string,
) error {
	logger.Infof("Initializing vegavisor in the %s", gen.userSettings.VisorHome)
	if err := vegacmd.InitVisor(visorBinary, gen.userSettings.VisorHome); err != nil {
		return fmt.Errorf(
			"failed to initialize vegavisor in %s: %w",
			gen.userSettings.VisorHome,
			err,
		)
	}
	logger.Info("Visor successfully initialized")

	logger.Infof("Initializing tendermint in the %s", gen.userSettings.TendermintHome)
	if err := vegacmd.InitTendermint(vegaBinary, gen.userSettings.TendermintHome); err != nil {
		return fmt.Errorf(
			"failed to initialize tendermint in %s: %w",
			gen.userSettings.TendermintHome,
			err,
		)
	}
	logger.Info("Tendermint successfully initialized")

	logger.Infof("Initializing vega in the %s", gen.userSettings.VegaHome)
	if err := vegacmd.InitVega(vegaBinary, gen.userSettings.VegaHome, vegacmd.VegaNodeFull); err != nil {
		return fmt.Errorf(
			"failed to initialize vega in %s: %w",
			gen.userSettings.VegaHome,
			err,
		)
	}
	logger.Info("Visor successfully initialized")

	logger.Infof("Initializing data-node n the %s", gen.userSettings.DataNodeHome)
	if err := vegacmd.InitDataNode(vegaBinary, gen.userSettings.DataNodeHome, gen.userSettings.MainnetChainId); err != nil {
		return fmt.Errorf(
			"failed to initialize data-node in %s: %w",
			gen.userSettings.DataNodeHome,
			err,
		)
	}
	logger.Info("Data-node successfully initialized")

	return nil
}