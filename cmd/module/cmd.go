// Package main is a module which serves the customsensor custom model.
package main

import (
	"context"
	// TODO: update if you're implementing a different interface
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/module"
	"go.viam.com/utils"

	// Import your local package "customsensor"
	// TODO: Update this path if your custom resource is in a different location,
	// or has a different name:
	ruddermotor "github.com/ChrisPullToRefresh/ruddermotor"
)

func main() {
	// NewLoggerFromArgs will create a logging.Logger at "DebugLevel" if
	// "--log-level=debug" is an argument in os.Args and at "InfoLevel" otherwise.
	// TODO: Change the name of the logger from customsensor to the name of the module your are creating
	utils.ContextualMain(mainWithArgs, module.NewLoggerFromArgs("ruddermotor"))
}

func mainWithArgs(ctx context.Context, args []string, logger logging.Logger) (err error) {
	myModule, err := module.NewModuleFromArgs(ctx, logger)
	if err != nil {
		return err
	}

	// Adds the preregistered sensor component API to the module for the new model.
	// TODO: Update the name of your package customsensor
	err = myModule.AddModelFromRegistry(ctx, motor.API, ruddermotor.Model)
	if err != nil {
		return err
	}

	err = myModule.Start(ctx)
	defer myModule.Close(ctx)
	if err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
