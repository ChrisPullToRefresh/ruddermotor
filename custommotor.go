// Package custommotor implements a motor
// TODO: rename if needed (i.e., custommotor)
package custummotor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	// TODO: update to the interface you'll implement
	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/operation"
	"go.viam.com/rdk/resource"

	"go.viam.com/utils"
)

// Here is where we define your new model's colon-delimited-triplet (viam-labs:go-module-templates-motor:custommotor)
// viam-labs = namespace, go-module-templates-motor = repo-name, custommotor = model name.
// TODO: Change model namespace, family (often the repo-name), and model. For more information see https://docs.viam.com/registry/create/#name-your-new-resource-model
var (
	Model            = resource.NewModel("pulltorefresh", "ruddermotor", "ruddermotor")
	errUnimplemented = errors.New("unimplemented")
)

type rudderState string

const (
	ccwRudderState     = "ccwRudder"
	cwRudderState      = "cwRudder"
	stoppedRudderState = "stoppedRudder"

	rudderCwPin  = "31"
	rudderCcwPin = "32"

	rudderStopPwmDutyCycle = 0
	rudderSlowPwmDutyCycle = 0.2
	rudderPwmFrequency     = 500
)

func init() {
	resource.RegisterComponent(motor.API, Model,
		// TODO: update to the interface you'll implement
		resource.Registration[motor.Motor, *Config]{
			Constructor: newCustomMotor,
		},
	)
}

// TODO: Change the Config struct to contain any values that you would like to be able to configure from the attributes field in the component
// configuration. For more information see https://docs.viam.com/build/configure/#components
type Config struct {
	Board                string `json:"board"`
	EncoderResetStraight string `json:"encoderResetStraight"`
}

// Validate validates the config and returns implicit dependencies.
// TODO: Change the Validate function to validate any config variables.
func (cfg *Config) Validate(path string) ([]string, error) {

	if cfg.Board == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "board")
	}

	// TODO: return implicit dependencies if needed as the first value
	return []string{}, nil
}

// Constructor for a custom motor that creates and returns a customMotor.
// TODO: update the customMotor struct and the initialization, and rename it
// if needed (i.e., newCustomMotor)
func newCustomMotor(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (motor.Motor, error) {
	// This takes the generic resource.Config passed down from the parent and converts it to the
	// model-specific (aka "native") Config structure defined above, making it easier to directly access attributes.
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	// Create a cancelable context for custom motor
	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	m := &customMotor{
		name:       rawConf.ResourceName(),
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		opMgr:      operation.NewSingleOperationManager(),
	}

	// TODO: If your custom component has dependencies, perform any checks you need to on them.

	// The Reconfigure() method changes the values on the customMotor based on the attributes in the component config
	if err := m.Reconfigure(ctx, deps, rawConf); err != nil {
		logger.Error("Error configuring module with ", rawConf)
		return nil, err
	}

	m.resetRudder()
	m.rs = stoppedRudderState

	return m, nil
}

// TODO: update the customMotor struct with any fields you require and
// rename the struct as needed (i.e., customMotor)
type customMotor struct {
	name   resource.Name
	logger logging.Logger
	cfg    *Config

	cancelCtx  context.Context
	cancelFunc func()
	mu         sync.Mutex
	opMgr      *operation.SingleOperationManager

	b        board.Board
	ers      sensor.Sensor
	rs       rudderState
	powerPct float64
}

// GoTo implements motor.Motor.
func (m *customMotor) GoTo(ctx context.Context, rpm float64, positionRevolutions float64, extra map[string]interface{}) error {
	return fmt.Errorf("GoTo not yet implemented")
}

// GoFor implements motor.Motor.
func (m *customMotor) GoFor(ctx context.Context, rpm float64, revolutions float64, extra map[string]interface{}) error {
	return fmt.Errorf("GoFor not yet implemented")
}

// IsMoving implements motor.Motor.
func (m *customMotor) IsMoving(context.Context) (bool, error) {
	return m.rs != stoppedRudderState, nil
}

// IsPowered implements motor.Motor.
func (m *customMotor) IsPowered(ctx context.Context, extra map[string]interface{}) (bool, float64, error) {
	isPowered := m.rs != stoppedRudderState
	powerPct := 0.0
	if isPowered {
		powerPct = m.powerPct
	}
	return isPowered, powerPct, nil
}

// Position implements motor.Motor.
func (m *customMotor) Position(ctx context.Context, extra map[string]interface{}) (float64, error) {
	return 0.0, fmt.Errorf("Position not yet implemented")
}

// Properties implements motor.Motor.
func (m *customMotor) Properties(ctx context.Context, extra map[string]interface{}) (motor.Properties, error) {
	return motor.Properties{}, fmt.Errorf("ResetZeroPosition not yet implemented")
}

// ResetZeroPosition implements motor.Motor.
func (m *customMotor) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	if (m.rs != ccwRudderState) && (m.rs != cwRudderState) {
		return fmt.Errorf("can only call ResetZeroPosition when turning. current rudder state = %v", m.rs)
	}
	newPowerPct := -m.powerPct

	m.SetPower(ctx, newPowerPct, nil)
	// TODO: implement as a go function and store the cancel function in custommotor
	m.mu.Lock()
	defer m.mu.Unlock()
	//for {
	readings, err := m.ers.Readings(ctx, nil)
	if err != nil {
		m.logger.Error(err)
		return err
	}
	m.logger.Infof("encoder set straight: %v", readings)
	return nil
	//}
}

func (m *customMotor) setPin(pinName string, high bool) {
	pin, err := m.b.GPIOPinByName(pinName)
	if err != nil {
		m.logger.Error(err)
		return
	}
	err = pin.Set(context.Background(), high, nil)
	if err != nil {
		m.logger.Error(err)
		return
	}
}

func (m *customMotor) setPwmFrequency(pinName string, freqHz uint) {
	pin, err := m.b.GPIOPinByName(pinName)
	if err != nil {
		m.logger.Error(err)
		return
	}
	err = pin.SetPWMFreq(m.cancelCtx, freqHz, nil)
	if err != nil {
		m.logger.Error(err)
		return
	}
}

func (m *customMotor) setPwmDutyCycle(pinName string, dutyCyclePct float64) {
	pin, err := m.b.GPIOPinByName(pinName)
	if err != nil {
		m.logger.Error(err)
		return
	}
	err = pin.SetPWM(m.cancelCtx, dutyCyclePct, nil)
	if err != nil {
		m.logger.Error(err)
		return
	}
}

func (m *customMotor) resetRudder() {
	/*
			await setPin(boardClient, rudderCwPin, false);
		  await setPin(boardClient, rudderCcwPin, false);

		  await setPwmFrequency(boardClient, rudderCwPin, rudderPwmFrequency);
		  await setPwmFrequency(boardClient, rudderCcwPin, rudderPwmFrequency);
	*/
	m.mu.Lock()
	defer m.mu.Unlock()

	m.setPin(rudderCwPin, false)
	m.setPin(rudderCcwPin, false)

	m.setPwmFrequency(rudderCwPin, rudderPwmFrequency)
	m.setPwmFrequency(rudderCcwPin, rudderPwmFrequency)
}

func (m *customMotor) stopRudder() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.setPwmDutyCycle(rudderCwPin, rudderStopPwmDutyCycle)
	m.setPwmDutyCycle(rudderCcwPin, rudderStopPwmDutyCycle)

	m.rs = stoppedRudderState
}

func iotaEqual(x, y float64) bool {
	iota := 0.001
	return math.Abs(x-y) <= iota
}

// SetPower implements motor.Motor.
// powerPct > 0 == raise == cw
func (m *customMotor) SetPower(ctx context.Context, powerPct float64, extra map[string]interface{}) error {
	m.opMgr.CancelRunning(ctx)

	if iotaEqual(powerPct, 0.0) {
		return m.Stop(ctx, nil)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var pin string
	if powerPct > 0 {
		pin = rudderCwPin
		m.rs = cwRudderState
	} else {
		pin = rudderCcwPin
		m.rs = ccwRudderState
	}
	newPowerPct := math.Abs(powerPct)
	m.setPwmDutyCycle(pin, newPowerPct)
	m.powerPct = newPowerPct

	return nil
}

// SetRPM implements motor.Motor.
func (m *customMotor) SetRPM(ctx context.Context, rpm float64, extra map[string]interface{}) error {
	return fmt.Errorf("SetRPM not yet implemeented")
}

// Stop implements motor.Motor.
func (m *customMotor) Stop(ctx context.Context, extra map[string]interface{}) error {
	m.opMgr.CancelRunning(ctx)

	m.stopRudder()
	return nil
}

// TODO: rename as needed (i.e., m customMotor)
func (m *customMotor) Name() resource.Name {
	return m.name
}

// Reconfigures the model. Most models can be reconfigured in place without needing to rebuild. If you need to instead create a new instance of the motor, throw a NewMustBuildError.
// TODO: rename as appropriate, i.e. m *customMotor
func (m *customMotor) Reconfigure(ctx context.Context, deps resource.Dependencies, conf resource.Config) error {
	m.opMgr.CancelRunning(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()

	// TODO: rename as appropriate (i.e., motorConfig)
	motorConfig, err := resource.NativeConfig[*Config](conf)
	if err != nil {
		m.logger.Warn("Error reconfiguring module with ", err)
		return err
	}

	m.name = conf.ResourceName()

	m.b, err = board.FromDependencies(deps, motorConfig.Board)
	if err != nil {
		return fmt.Errorf("unable to get board %v for %v", motorConfig.Board, m.name)
	}
	m.logger.Info("board is now configured to ", m.b.Name())

	m.ers, err = sensor.FromDependencies(deps, motorConfig.EncoderResetStraight)
	if err != nil {
		return fmt.Errorf("unable to get sensor %v for %v", motorConfig.EncoderResetStraight, m.name)
	}
	m.logger.Info("encoder-resetstraight is now configured to ", m.ers.Name())

	return nil
}

// DoCommand is a place to add additional commands to extend the motor API. This is optional.
// TODO: rename as appropriate (i.e., motorConfig)
func (m *customMotor) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	m.logger.Error("DoCommand method unimplemented")
	return nil, errUnimplemented
}

// Close closes the underlying generic.
// TODO: rename as appropriate (i.e., motorConfig)
func (m *customMotor) Close(ctx context.Context) error {
	err := m.Stop(ctx, nil)
	m.cancelFunc()
	return err
}
