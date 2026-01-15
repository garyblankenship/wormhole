package wormhole

import (
	"math"
	"time"
)

// PIDConfig holds PID controller tuning parameters
type PIDConfig struct {
	Kp float64 // Proportional gain
	Ki float64 // Integral gain
	Kd float64 // Derivative gain

	// Anti-windup: limit integral term accumulation
	MaxIntegral float64
	MinIntegral float64

	// Output limits
	MaxOutput float64
	MinOutput float64
}

// DefaultPIDConfig returns sensible defaults for concurrency control
func DefaultPIDConfig() PIDConfig {
	return PIDConfig{
		Kp: 0.5,    // Moderate proportional response
		Ki: 0.1,    // Slow integral correction
		Kd: 0.05,   // Dampen oscillations

		MaxIntegral: 10.0,
		MinIntegral: -10.0,
		MaxOutput:   0.5,  // Max 50% capacity change per adjustment
		MinOutput:  -0.5,  // Max 50% reduction per adjustment
	}
}

// PIDController implements a PID control algorithm
type PIDController struct {
	config PIDConfig

	// State
	integralError float64
	lastError     float64
	lastTime      time.Time
	initialized   bool
}

// NewPIDController creates a new PID controller
func NewPIDController(config PIDConfig) *PIDController {
	return &PIDController{
		config: config,
	}
}

// Compute calculates the control output based on error
func (p *PIDController) Compute(setpoint, measurement, dt time.Duration) float64 {
	if !p.initialized {
		p.lastTime = time.Now()
		p.initialized = true
		return 0.0
	}

	// Normalized error: (actual - target) / target
	error := float64(measurement-setpoint) / float64(setpoint)

	// Calculate time delta in seconds
	dtSec := dt.Seconds()
	if dtSec <= 0 {
		dtSec = 1.0 // Default to 1s if invalid
	}

	// Proportional term
	proportional := p.config.Kp * error

	// Integral term with anti-windup
	p.integralError += error * dtSec
	p.integralError = math.Max(p.config.MinIntegral,
		math.Min(p.config.MaxIntegral, p.integralError))
	integral := p.config.Ki * p.integralError

	// Derivative term
	derivative := 0.0
	if dtSec > 0 {
		derivative = p.config.Kd * (error - p.lastError) / dtSec
	}

	p.lastError = error

	// Compute output
	output := proportional + integral + derivative

	// Clamp output
	output = math.Max(p.config.MinOutput,
		math.Min(p.config.MaxOutput, output))

	return output
}

// Reset clears the controller state
func (p *PIDController) Reset() {
	p.integralError = 0.0
	p.lastError = 0.0
	p.initialized = false
}