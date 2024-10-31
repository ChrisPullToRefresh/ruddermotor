// Package custommotor implements a motor
// TODO: rename if needed (i.e., custommotor)
package custummotor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	// TODO: update to the interface you'll implement
	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/encoder"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/logging"
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

	rudderStopPwmDutyCycle = 0.0
	rudderPwmDutyCycle     = 0.5
	//rudderFastPwmDutyCycle               = 1.0
	rudderSmallTurnTime                  = time.Millisecond * 500
	rudderBigTurnTime                    = time.Millisecond * 1000
	rudderResetZeroTimeOut               = time.Millisecond * 1500
	rudderResetZeroPollPauseMilliseconds = 10
	rudderPwmFrequency                   = 500
	// ResetZeroPosition will pause for this length of time before returning
	// to zero - this is the key of the value passed to the function
	pauseBeforeReset      = "pauseBeforeReset"
	pauseBeforeResetValue = 500

	rudderCommandTurnThenCenter = "turnThenCenter"
	rudderCommandSmallLeft      = "smallLeft"
	rudderCommandSmallRight     = "smallRight"
	rudderCommandBigLeft        = "bigLeft"
	rudderCommandBigRight       = "bigRight"
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
	ResetPin             string `json:"resetPin"`
}

// Validate validates the config and returns implicit dependencies.
// TODO: Change the Validate function to validate any config variables.
func (cfg *Config) Validate(path string) ([]string, error) {

	if cfg.Board == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "board")
	}

	if cfg.EncoderResetStraight == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "encoderResetStraight")
	}

	if cfg.ResetPin == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "resetPin")
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

	b        board.Board
	ers      encoder.Encoder
	rs       rudderState
	powerPct float64
}

// GoTo implements motor.Motor.
func (m *customMotor) GoTo(ctx context.Context, rpm float64, positionRevolutions float64, extra map[string]interface{}) error {
	return errUnimplemented
}

// GoFor implements motor.Motor.
func (m *customMotor) GoFor(ctx context.Context, rpm float64, revolutions float64, extra map[string]interface{}) error {
	return errUnimplemented
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
	return 0.0, errUnimplemented
}

// Properties implements motor.Motor.
func (m *customMotor) Properties(ctx context.Context, extra map[string]interface{}) (motor.Properties, error) {
	return motor.Properties{}, errUnimplemented
}

// ResetZeroPosition implements motor.Motor.
func (m *customMotor) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	var pause = time.Millisecond * 0
	for key, value := range extra {
		switch key {
		case pauseBeforeReset:
			pauseMilliseconds, ok := value.(int)
			if !ok {
				return fmt.Errorf("unparseable int argument for extra argument %v = %v", key, value)
			}
			pause = time.Millisecond * time.Duration(pauseMilliseconds)
		default:
			return fmt.Errorf("unknown extra key = %v", key)
		}
	}

	m.logger.Infof("Begin ResetZeroPosition")
	if (m.rs != ccwRudderState) && (m.rs != cwRudderState) {
		return fmt.Errorf("can only call ResetZeroPosition when turning. current rudder state = %v", m.rs)
	}
	m.logger.Infof("current power: %v", m.powerPct)
	newPowerPct := m.powerPct
	signNewPowerPct := 1.0
	if m.rs == cwRudderState {
		signNewPowerPct = -1.0
	}
	newPowerPct *= signNewPowerPct
	m.logger.Infof("new power: %v", newPowerPct)
	m.Stop(ctx, nil)
	time.Sleep(pause)
	m.SetPower(ctx, newPowerPct, nil)
	// TODO: implement as a go function and store the cancel function in custommotor
	m.mu.Lock()
	startTicks := -1.0
	timer := time.After(rudderResetZeroTimeOut)
	for {
		select {
		case <-timer:
			m.mu.Unlock()
			m.Stop(ctx, nil)
			return fmt.Errorf("timed out of ResetZeroPosition after %v milliseconds", rudderResetZeroTimeOut)
		default:
			ticks, _, err := m.ers.Position(ctx, encoder.PositionTypeTicks, nil)
			if err != nil {
				m.logger.Error(err)
				m.mu.Unlock()
				m.Stop(ctx, nil)
				return err
			}
			if startTicks < 0 {
				startTicks = ticks
				m.logger.Infof("encoder set straight startTicks: %v", startTicks)
			} else if startTicks != ticks {
				m.logger.Infof("encoder set straight end turn ticks: %v", startTicks)
				m.mu.Unlock()
				m.Stop(ctx, nil)
				return nil
			}
			time.Sleep(time.Millisecond * rudderResetZeroPollPauseMilliseconds)
		}
	}
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

	m.ers, err = encoder.FromDependencies(deps, motorConfig.EncoderResetStraight)
	if err != nil {
		return fmt.Errorf("unable to get encoder %v for %v", motorConfig.EncoderResetStraight, m.name)
	}
	m.logger.Info("encoder-resetstraight is now configured to ", m.ers.Name())

	pin, err := m.b.GPIOPinByName(m.cfg.ResetPin)
	if err != nil {
		return fmt.Errorf("unable to get resets encoder pin %v for %v", m.cfg.ResetPin, m.name)
	}
	pin.Set(ctx, false, nil)
	m.logger.Info("Reset encoder pin %v to low", m.cfg.ResetPin)

	return nil
}

// DoCommand is a place to add additional commands to extend the motor API. This is optional.
// TODO: rename as appropriate (i.e., motorConfig)
func (m *customMotor) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	for key, value := range cmd {
		switch key {
		// "turnThenCenter":"smallLeft"
		case rudderCommandTurnThenCenter:
			command := value.(string)
			powerPct := 0.0
			rudderTurnTime := time.Millisecond * 0
			switch command {
			case rudderCommandSmallLeft:
				powerPct = -rudderPwmDutyCycle
				rudderTurnTime = rudderSmallTurnTime
			case rudderCommandSmallRight:
				powerPct = rudderPwmDutyCycle
				rudderTurnTime = rudderSmallTurnTime
			case rudderCommandBigLeft:
				powerPct = -rudderPwmDutyCycle
				rudderTurnTime = rudderBigTurnTime
			case rudderCommandBigRight:
				powerPct = rudderPwmDutyCycle
				rudderTurnTime = rudderBigTurnTime
			default:
				return nil, fmt.Errorf("unknown DoCommand value for %v = %v", key, value)
			}
			m.SetPower(ctx, powerPct, nil)
			time.Sleep(rudderTurnTime)
			extra := make(map[string]interface{})
			extra[pauseBeforeReset] = pauseBeforeResetValue
			m.ResetZeroPosition(ctx, 0, extra)
			return nil, nil
		default:
			return nil, fmt.Errorf("unknown DoCommand key = %v ", key)
		}
	}
	return nil, fmt.Errorf("unknown DoCommand command map: %v", cmd)
}

// Close closes the underlying generic.
// TODO: rename as appropriate (i.e., motorConfig)
func (m *customMotor) Close(ctx context.Context) error {
	err := m.Stop(ctx, nil)
	m.cancelFunc()
	return err
}
