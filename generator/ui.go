package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/rodaine/table"
	input "github.com/tcnksm/go-input"

	"github.com/daniel1302/vega-asistant/types"
	"github.com/daniel1302/vega-asistant/utils"
)

type YesNoAnswer string

const (
	AnswerYes YesNoAnswer = "Yes"
	AnswerNo  YesNoAnswer = "No"
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

func AskPath(ui *input.UI, name, defaultValue string) (string, error) {
	response, err := ui.Ask(fmt.Sprintf("What is your %s", name), &input.Options{
		Default:  defaultValue,
		Required: true,
		Loop:     true,
		ValidateFunc: func(s string) error {
			if utils.FileExists(s) {
				return fmt.Errorf(
					"given path exists on your fs, remove this file or provide another directory",
				)
			}

			return nil
		},
	})
	if err != nil {
		return "", types.NewInputError(err)
	}

	return response, nil
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

func AskYesNo(ui *input.UI, question string, defaultAnswer YesNoAnswer) (YesNoAnswer, error) {
	answer, err := ui.Ask(question,
		&input.Options{
			Default:  string(defaultAnswer),
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
		return defaultAnswer, fmt.Errorf("failed to ask for yes/no: %w", err)
	}

	if strings.ToLower(answer) == "yes" {
		return AnswerYes, nil
	}

	return AnswerNo, nil
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
	tbl.AddRow("Vega Version", settings.MainnetVersion)
	tbl.AddRow("Vega Chain ID", settings.MainnetChainId)

	tbl.Print()
	fmt.Println("")
}
