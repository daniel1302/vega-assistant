package datanode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	input "github.com/tcnksm/go-input"

	"github.com/daniel1302/vega-assistant/types"
	"github.com/daniel1302/vega-assistant/uilib"
	"github.com/daniel1302/vega-assistant/vega"
)

func SelectStartupMode(ui *input.UI, defaultValue StartupMode) (*StartupMode, error) {
	const msg = `How do you want to start your data-node?

  - Starting from block 0 - Starts the node from the genesis binary, replays all the blocks and 
                            does all of the protocol upgrades automatically.
        * Depending on network age it can takes up to several days to catch your node up.
        * Full network history is available on your node.
  
  - Starting from network history - Start the node from latest binary, and download all required 
                                    informations from the running network. 
        * It takes up to several minutes.
        * No historical data is available on your node.`
	response, err := ui.Select(
		msg,
		[]string{string(StartFromBlock0), string(StartFromNetworkHistory)},
		&input.Options{
			Default:  string(defaultValue),
			Loop:     true,
			Required: true,
		},
	)
	if err != nil {
		return nil, types.NewInputError(err)
	}

	result := StartFromNetworkHistory
	if response == string(StartFromBlock0) {
		result = StartFromBlock0
	}

	return &result, nil
}

func AskNetworkHistoryEnabled(ui *input.UI) (uilib.YesNoAnswer, error) {
	fmt.Println(`
The network history is the data node feature allowing start a new data node 
from one of the latest blocks and sync data faster. If you enable it, you can 
share your node peer info with other people and allow then to use your node 
as the data feed source. It requires more disk space on your server as copy 
of all the data persist on the disk.

Don't you know if you need the network history? Do not enable it.`)
	return uilib.AskYesNo(ui, "do you want to enable network-history feature?", uilib.AnswerNo)
}

func AskRetentionPolicy(ui *input.UI) (string, error) {
	ui.Ask("Retention policy. Possible values: standard, archival, 1 (day|month|year), 3 (days|months|years), etc...", &input.Options{
		Default:  "standard",
		Required: true,
		Loop:     true,
		ValidateFunc: func(s string) error {
			if !vega.IsRetentionPolicyValid(s) {
				return fmt.Errorf("invalid retention policy")
			}

			return nil
		},
	})
}

func AskSQLCredentials(
	ui *input.UI,
	defaultValue types.SQLCredentials,
	checkFunc func(types.SQLCredentials) error,
) (*types.SQLCredentials, error) {
	var (
		dbHost string
		dbUser string
		dbPort int
		dbPass string
		dbName string

		err error
	)

	fmt.Println("PostgreSQL server must be running and you MUST install the TimescaleDB v2.8.0")
	for {
		dbHost, err = ui.Ask("PostgreSQL host for the data-node", &input.Options{
			Default:  defaultValue.Host,
			Required: true,
			Loop:     true,
		})
		if err != nil {
			return nil, types.NewInputError(fmt.Errorf("failed to get postgresql host: %w", err))
		}

		dbPortStr, err := ui.Ask("PostgreSQL port for the data-node", &input.Options{
			Default:  fmt.Sprintf("%d", defaultValue.Port),
			Required: true,
			Loop:     true,
			ValidateFunc: func(s string) error {
				if _, err := strconv.Atoi(s); err != nil {
					return fmt.Errorf("port must be numeric: %w", err)
				}

				return nil
			},
		})
		if err != nil {
			return nil, types.NewInputError(fmt.Errorf("failed to get postgresql port: %w", err))
		}

		dbPort, err = strconv.Atoi(dbPortStr)
		if err != nil {
			return nil, types.NewInputError(fmt.Errorf("port must be numeric: %w", err))
		}

		dbUser, err = ui.Ask("PostgreSQL user name for the data-node", &input.Options{
			Default:  defaultValue.User,
			Required: true,
			Loop:     true,
		})

		if err != nil {
			return nil, fmt.Errorf("failed to ask for database user name: %w", err)
		}

		dbPass, err = ui.Ask("PostgreSQL password for the given username", &input.Options{
			Default:  defaultValue.Pass,
			Required: true,
			Loop:     true,
		})

		if err != nil {
			return nil, fmt.Errorf("failed to ask for database password: %w", err)
		}

		dbName, err = ui.Ask("PostgreSQL database name for the data-node", &input.Options{
			Default:  defaultValue.DatabaseName,
			Required: true,
			Loop:     true,
		})

		if err != nil {
			return nil, fmt.Errorf("failed to ask for database name: %w", err)
		}

		if err := checkFunc(types.SQLCredentials{
			Host:         dbHost,
			User:         dbUser,
			Port:         dbPort,
			Pass:         dbPass,
			DatabaseName: dbName,
		}); err != nil {
			tryAgain, err := ui.Ask(
				fmt.Sprintf(
					"Cannot connect to the data base with given credentials(%s). Try again? (Yes/No)",
					err.Error(),
				),
				&input.Options{
					Default:  "Yes",
					Required: true,
					Loop:     true,
					ValidateFunc: func(s string) error {
						normalizedResponse := strings.ToLower(s)
						if normalizedResponse != "yes" && normalizedResponse != "no" {
							return fmt.Errorf("invalid response; got %s, expected Yes or No", s)
						}
						return nil
					},
				},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to ask for try-again: %w", err)
			}

			if strings.ToLower(tryAgain) == "yes" {
				continue
			}
		}

		break
	}

	return &types.SQLCredentials{
		Host:         dbHost,
		User:         dbUser,
		Port:         dbPort,
		Pass:         dbPass,
		DatabaseName: dbName,
	}, nil
}

func printSummary(settings GenerateSettings) {
	fmt.Println("\n Summary:\n")
	headerFmt := color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt := color.New(color.FgYellow).SprintfFunc()

	tbl := table.New("Parameter", "Value")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
	if settings.Mode == StartFromBlock0 {
		tbl.AddRow("Mode", "Start from block 0")
	} else {
		tbl.AddRow("Mode", "Start from Network History")
	}
	tbl.AddRow("Visor Home", settings.VisorHome)
	tbl.AddRow("Vega Home", settings.VegaHome)
	tbl.AddRow("Tendermint Home", settings.TendermintHome)
	tbl.AddRow("SQL Host", settings.SQLCredentials.Host)
	tbl.AddRow("SQL Port", settings.SQLCredentials.Port)
	tbl.AddRow("SQL User", settings.SQLCredentials.User)
	tbl.AddRow(
		"SQL Password",
		fmt.Sprintf(
			"%c***%c",
			settings.SQLCredentials.Pass[0],
			settings.SQLCredentials.Pass[len(settings.SQLCredentials.Pass)-1],
		),
	)
	tbl.AddRow("SQL Database Name", settings.SQLCredentials.DatabaseName)
	tbl.AddRow("Vega Version", settings.VegaBinaryVersion)
	tbl.AddRow("Vega Chain ID", settings.VegaChainId)

	tbl.Print()
	fmt.Println("")
}

func PrintInstructions(visorHome string) {
	fmt.Printf(`
    The data node is initialized. You can now start it with the following command:

      %s/visor run --home %s

    After node is running and it is moving block forwards do not forget to execute the following command. It is very important otherwise your node will be wiped every restart!

      vega-assistant setup post-start

    You can also setup systemd service if you running your node on LINUX with the following command:

      sudo vega-assistant setup systemd --visor-home %s

    You must call the above command as a root user otherwise you will get instructions for manual systemd setup.`, visorHome, visorHome, visorHome)
}
