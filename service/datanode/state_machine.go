package datanode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pg "github.com/go-pg/pg/v11"
	"github.com/tcnksm/go-input"
	"go.uber.org/zap"
	"golang.org/x/mod/semver"

	"github.com/pelletier/go-toml"

	"github.com/daniel1302/vega-assistant/network"
	"github.com/daniel1302/vega-assistant/types"
	"github.com/daniel1302/vega-assistant/uilib"
	"github.com/daniel1302/vega-assistant/utils"
	"github.com/daniel1302/vega-assistant/vega"
	"github.com/daniel1302/vega-assistant/vegaapi"
)

type (
	State       int
	StartupMode string
)

const (
	StartFromBlock0         StartupMode = "start-from-block-0"
	StartFromNetworkHistory StartupMode = "startup-from-network-history"
)

const (
	StateSelectStartupMode State = iota
	StateSelectHowManyBlockToSync
	StateSelectNetworkHistoryEnabled
	SelectDataRetention
	StateSelectVisorHome
	StateExistingVisorHome
	StateSelectVegaHome
	StateExistingVegaHome
	StateSelectTendermintHome
	StateExistingTendermintHome
	StateGetSQLCredentials
	StateCheckLatestVersion
	StateSummary
)

type StateMachine struct {
	CurrentState State
	Settings     GenerateSettings

	logger *zap.SugaredLogger
}

type GenerateSettings struct {
	Mode StartupMode

	NonInteractive              bool   `toml:"non-interactive"`
	NetworkHistoryEnabled       bool   `toml:"network-history-enabled"`
	DataRetention               string `toml:"data-retention"`
	VisorHome                   string `toml:"visor-home"`
	VegaHome                    string `toml:"vega-home"`
	TendermintHome              string `toml:"tendermint-home"`
	DataNodeHome                string `toml:"data-node-home"`
	VisorBinaryVersion          string
	VegaBinaryVersion           string
	VegaChainId                 string
	NetworkHistoryMinBlockCount int                  `toml:"network-history-min-block-count"`
	RemoveExistingFiles         bool                 `toml:"remove-existing-file"`
	SQLCredentials              types.SQLCredentials `toml:"sql-credentials"`
}

func DefaultGenerateSettings() *GenerateSettings {
	return &GenerateSettings{
		NonInteractive:      false,
		Mode:                StartFromNetworkHistory,
		VisorHome:           filepath.Join(utils.CurrentUserHomePath(), "vegavisor_home"),
		VegaHome:            filepath.Join(utils.CurrentUserHomePath(), "vega_home"),
		TendermintHome:      filepath.Join(utils.CurrentUserHomePath(), "tendermint_home"),
		RemoveExistingFiles: false,

		SQLCredentials: types.SQLCredentials{
			Host:         "localhost",
			User:         "vega",
			Pass:         "vega",
			Port:         5432,
			DatabaseName: "vega",
		},
	}
}

func ReadGeneratorSettingsFromFile(filePath string) (*GenerateSettings, error) {
	tomlTree, err := toml.LoadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w")
	}

	result := &GenerateSettings{}
	if err := tomlTree.Unmarshal(result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %w", err)
	}

	return result, nil
}

func NewStateMachine(logger *zap.SugaredLogger, config GenerateSettings) StateMachine {
	return StateMachine{
		logger:       logger,
		CurrentState: StateSelectStartupMode,
		Settings:     config,
	}
}

func (state StateMachine) Dump() string {
	result, err := json.MarshalIndent(state, "", "    ")
	if err != nil {
		return ""
	}

	return string(result)
}

func (state *StateMachine) Run(ui *input.UI, networkConfig network.NetworkConfig) error {
STATE_RUN:
	for {
		switch state.CurrentState {
		case StateSelectStartupMode:
			if state.Settings.NonInteractive {
				state.logger.Info("NonInteractive: Using %s mode", state.Settings.Mode)
			} else {
				mode, err := SelectStartupMode(ui, state.Settings.Mode)
				if err != nil {
					return fmt.Errorf("failed selecting startup mode: %w", err)
				}
				state.Settings.Mode = *mode
			}

			if state.Settings.Mode == StartFromNetworkHistory {
				state.CurrentState = StateSelectHowManyBlockToSync
			} else {
				state.CurrentState = StateSelectNetworkHistoryEnabled
			}

		case StateSelectHowManyBlockToSync:
			if state.Settings.NonInteractive {
				state.logger.Info("NonInteractive: Will sync %d blocks from the network history", state.Settings.NetworkHistoryMinBlockCount)
				state.CurrentState = StateSelectNetworkHistoryEnabled

				continue
			}

			networkHistoryMinBlockCount, err := uilib.AskInt(ui, "minimum blocks to sync from the network history", 10000)
			if err != nil {
				return fmt.Errorf("failed getting minimum blocks to sync from the network history: %w", err)
			}
			state.Settings.NetworkHistoryMinBlockCount = networkHistoryMinBlockCount
			state.CurrentState = StateSelectNetworkHistoryEnabled

		case StateSelectNetworkHistoryEnabled:
			if state.Settings.NonInteractive {
				if state.Settings.NetworkHistoryEnabled {
					state.logger.Info("Network history will be enabled")
				} else {
					state.logger.Info("Network history will be disabled")
				}
				state.CurrentState = SelectDataRetention

				continue
			}

			enabledNetworkHistoryResponse, err := AskNetworkHistoryEnabled(ui)
			if err != nil {
				return fmt.Errorf("failed getting response to enable network history: %w", err)
			}
			state.Settings.NetworkHistoryEnabled = enabledNetworkHistoryResponse == uilib.AnswerYes

			state.CurrentState = SelectDataRetention

		case SelectDataRetention:
			if state.Settings.NonInteractive {
				if !vega.IsRetentionPolicyValid(state.Settings.DataRetention) {
					state.logger.Info("Data node retention: forever")
				} else {
					state.logger.Infof("Data node retention: %s", state.Settings.DataRetention)
				}

				state.CurrentState = StateSelectVisorHome
				continue
			}

			retentionPolicy, err := AskRetentionPolicy(ui)
			if err != nil {
				return fmt.Errorf("failed getting retention policy: %w", err)
			}
			state.Settings.DataRetention = retentionPolicy

		case StateSelectVisorHome:
			if state.Settings.NonInteractive {
				state.logger.Info("NonInteractive: Using %s for vegavisor home", state.Settings.VisorHome)
			} else {
				visorHome, err := uilib.AskPath(ui, "vegavisor home", state.Settings.VisorHome)
				if err != nil {
					return fmt.Errorf("failed getting vegavisor home: %w", err)
				}

				state.Settings.VisorHome = visorHome
			}

			if utils.FileExists(state.Settings.VisorHome) {
				state.CurrentState = StateExistingVisorHome
			} else {
				state.CurrentState = StateSelectVegaHome
			}

		case StateExistingVisorHome:
			if state.Settings.NonInteractive {
				if !state.Settings.RemoveExistingFiles {
					return fmt.Errorf("cannot remove existing visor home: non-interactive mode is enabled and config flag 'remove-existing-file' is disabled: provide different vegavisor home in the config or remove it manually")
				}
				state.logger.Info("NonInteractive: Will remove vegavisor home: %s", state.Settings.VisorHome)
			} else {
				removeAnswer, err := uilib.AskRemoveExistingFile(ui, state.Settings.VisorHome, uilib.AnswerYes)
				if err != nil {
					return fmt.Errorf("failed to get answer for remove existing visor home: %w", err)
				}

				if removeAnswer == uilib.AnswerNo {
					return fmt.Errorf("visor home exists. You must provide different visor home or remove it")
				}
			}

			if err := os.RemoveAll(state.Settings.VisorHome); err != nil {
				return fmt.Errorf("failed to remove vegavisor home: %w", err)
			}

			state.CurrentState = StateSelectVegaHome

		case StateSelectVegaHome:
			if state.Settings.NonInteractive {
				state.logger.Infof("NonInteractive: Using %s for vega home", state.Settings.VegaHome)

				state.Settings.DataNodeHome = state.Settings.VegaHome
			} else {
				vegaHome, err := uilib.AskPath(ui, "vega home", state.Settings.VegaHome)
				if err != nil {
					return fmt.Errorf("failed getting vega home: %w", err)
				}
				state.Settings.VegaHome = vegaHome
				state.Settings.DataNodeHome = vegaHome
			}

			if utils.FileExists(state.Settings.VegaHome) {
				state.CurrentState = StateExistingVegaHome
			} else {
				state.CurrentState = StateSelectTendermintHome
			}

		case StateExistingVegaHome:
			if state.Settings.NonInteractive {
				if !state.Settings.RemoveExistingFiles {
					return fmt.Errorf("cannot remove existing vega home: non-interactive mode is enabled and config flag 'remove-existing-file' is disabled: provide different vega home in the config or remove it manually")
				}
				state.logger.Infof("NonInteractive: Will remove vega home: %s", state.Settings.VegaHome)
			} else {
				removeAnswer, err := uilib.AskRemoveExistingFile(ui, state.Settings.VegaHome, uilib.AnswerYes)
				if err != nil {
					return fmt.Errorf("failed to get answer for remove existing vega home: %w", err)
				}

				if removeAnswer == uilib.AnswerNo {
					return fmt.Errorf("vega home exists. You must provide different vega home or remove it")
				}
			}

			if err := os.RemoveAll(state.Settings.VegaHome); err != nil {
				return fmt.Errorf("failed to remove vega home: %w", err)
			}

			state.CurrentState = StateSelectTendermintHome

		case StateSelectTendermintHome:
			if state.Settings.NonInteractive {
				state.logger.Infof("NonInteractive: Using %s for tendermint home", state.Settings.TendermintHome)
			} else {
				tendermintHome, err := uilib.AskPath(ui, "tendermint home", state.Settings.TendermintHome)
				if err != nil {
					return fmt.Errorf("failed getting tendermint home: %w", err)
				}
				state.Settings.TendermintHome = tendermintHome
			}

			if utils.FileExists(state.Settings.TendermintHome) {
				state.CurrentState = StateExistingTendermintHome
			} else {
				state.CurrentState = StateGetSQLCredentials
			}

		case StateExistingTendermintHome:
			if state.Settings.NonInteractive {
				if !state.Settings.RemoveExistingFiles {
					return fmt.Errorf("cannot remove existing tendermint home: non-interactive mode is enabled and config flag 'remove-existing-file' is disabled: provide different tendermint home in the config or remove it manually")
				}
				state.logger.Infof("NonInteractive: Will remove vega home: %s", state.Settings.VegaHome)
			} else {
				removeAnswer, err := uilib.AskRemoveExistingFile(ui, state.Settings.TendermintHome, uilib.AnswerYes)
				if err != nil {
					return fmt.Errorf("failed to get answer for remove existing tendermint home: %w", err)
				}

				if removeAnswer == uilib.AnswerNo {
					return fmt.Errorf("tendermint home exists. You must provide different tendermint home or remove it")
				}
			}

			if err := os.RemoveAll(state.Settings.TendermintHome); err != nil {
				return fmt.Errorf("failed to remove tendermint home: %w", err)
			}

			state.CurrentState = StateGetSQLCredentials

		case StateGetSQLCredentials:
			if state.Settings.NonInteractive {
				state.logger.Infof(
					"NonInteractive: Using provided SQL settings: User(%s), Password(***), Host(%s), Port(%d), DbName(%s)",
					state.Settings.SQLCredentials.User,
					state.Settings.SQLCredentials.Host,
					state.Settings.SQLCredentials.Port,
					state.Settings.SQLCredentials.DatabaseName,
				)

				if err := checkSQLCredentials(state.Settings.SQLCredentials); err != nil {
					return fmt.Errorf("failed to check sql credentials: %w", err)
				}

				state.CurrentState = StateCheckLatestVersion
				continue
			}

			sqlCredentials, err := AskSQLCredentials(ui, state.Settings.SQLCredentials, checkSQLCredentials)
			if err != nil {
				return fmt.Errorf("failed getting sql credentials: %w", err)
			}
			state.Settings.SQLCredentials = *sqlCredentials
			state.CurrentState = StateCheckLatestVersion

		case StateCheckLatestVersion:
			statisticsResponse, err := vegaapi.Statistics(networkConfig.DataNodesRESTUrls)
			if err != nil {
				return fmt.Errorf("failed to get response for the /statistics endpoint from the mainnet servers: %w", err)
			}
			if state.Settings.Mode == StartFromBlock0 {
				state.Settings.VegaBinaryVersion = networkConfig.GenesisVersion
				state.Settings.VisorBinaryVersion = networkConfig.LowestVisorVersion
			} else {
				state.Settings.VegaBinaryVersion = statisticsResponse.Statistics.AppVersion
				state.Settings.VisorBinaryVersion = statisticsResponse.Statistics.AppVersion
			}

			state.Settings.VegaChainId = statisticsResponse.Statistics.ChainID
			state.CurrentState = StateSummary

		case StateSummary:
			printSummary(state.Settings)

			if state.Settings.NonInteractive {
				state.logger.Info("NonInteractive: Moving to installation steps")

				break STATE_RUN
			}

			correctResponse, err := uilib.AskYesNo(ui, "Is it correct?", uilib.AnswerYes)
			if err != nil {
				return fmt.Errorf("failed asking for correct summary: %w", err)
			}

			if correctResponse == uilib.AnswerNo {
				state.CurrentState = StateSelectStartupMode
				break
			}

			break STATE_RUN
		}
	}
	return nil
}

func checkSQLCredentials(creds types.SQLCredentials) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db := pg.Connect(&pg.Options{
		Addr:     fmt.Sprintf("%s:%d", creds.Host, creds.Port),
		User:     creds.User,
		Password: creds.Pass,
		Database: creds.DatabaseName,
	})
	defer db.Close(ctx)

	var n int
	_, err := db.QueryOne(ctx, pg.Scan(&n), "SELECT 1")
	if err != nil {
		return err
	}

	var timescaleVersion string
	_, err = db.QueryOne(
		ctx,
		pg.Scan(&timescaleVersion),
		`SELECT installed_version AS extversion FROM pg_available_extensions WHERE name = 'timescaledb'
		UNION ALL
		SELECT extversion AS extversion FROM pg_extension WHERE extname = 'timescaledb'
		LIMIT 1;`,
	)
	if err != nil {
		return fmt.Errorf("failed to check timescale extension version: %w", err)
	}

	if !strings.HasPrefix(timescaleVersion, "v") {
		timescaleVersion = fmt.Sprintf("v%s", timescaleVersion)
	}

	if semver.Compare(timescaleVersion, "v2.8.0") != 0 {
		return fmt.Errorf(
			"Vega support only timescale v2.8.0. Installed version is %s",
			timescaleVersion,
		)
	}

	return nil
}
