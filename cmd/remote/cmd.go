package main

import (
	"context"
	"os"
	"strconv"

	"go.viam.com/rdk/module"
	// TODO: update to the interface you'll implement
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/config"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	robotimpl "go.viam.com/rdk/robot/impl"
	"go.viam.com/rdk/robot/web"
	rdkutils "go.viam.com/rdk/utils"
	"go.viam.com/utils"

	// TODO: update to your project
	ruddermotor "github.com/ChrisPullToRefresh/ruddermotor"
)

func main() {
	// NewLoggerFromArgs will create a logging.Logger at "DebugLevel" if
	// "--log-level=debug" is an argument in os.Args and at "InfoLevel" otherwise.
	utils.ContextualMain(mainWithArgs, module.NewLoggerFromArgs("ruddermotor"))
}

func mainWithArgs(ctx context.Context, args []string, logger logging.Logger) (err error) {

	netconfig := config.NetworkConfig{}
	netconfig.BindAddress = "0.0.0.0:8083"

	if err := netconfig.Validate(""); err != nil {
		return err
	}

	pinInt, err := strconv.Atoi(os.Args[4])
	if err != nil {
		return err
	}
	logger.Infof("Reset pin for magnet encode set to %v", pinInt)

	// Update the Attributes and ConvertedAttributes with the attributes your modular resource should receive
	conf := &config.Config{
		Network: netconfig,
		Components: []resource.Config{
			{
				Name:  os.Args[1],
				API:   motor.API,
				Model: ruddermotor.Model,
				Attributes: rdkutils.AttributeMap{
					"board":                os.Args[2],
					"encoderResetStraight": os.Args[3],
					"resetPin":             os.Args[4],
				},
				ConvertedAttributes: &ruddermotor.Config{
					Board:                os.Args[2],
					EncoderResetStraight: os.Args[3],
					ResetPin:             os.Args[4],
				},
			},
		},
	}

	myRobot, err := robotimpl.New(ctx, conf, logger)
	if err != nil {
		return err
	}

	return web.RunWebWithConfig(ctx, myRobot, conf, logger)
}
