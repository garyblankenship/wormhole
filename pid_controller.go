package wormhole

import (
	"math"
	"time"
)

type pidConfig struct {
	kp float64
	ki float64
	kd float64

	maxIntegral float64
	minIntegral float64
	maxOutput   float64
	minOutput   float64
}

func defaultPIDConfig() pidConfig {
	return pidConfig{
		kp: 0.5, // Moderate proportional response
		// ki scaled so ki*maxIntegral stays within maxOutput (0.05*10.0=0.5);
		// the previous 0.1 let the integral term alone saturate output,
		// causing bang-bang (always +/-50%) instead of a graduated response.
		ki: 0.05,
		kd: 0.05, // Dampen oscillations

		maxIntegral: 10.0,
		minIntegral: -10.0,
		maxOutput:   0.5,  // Max 50% capacity change per adjustment
		minOutput:   -0.5, // Max 50% reduction per adjustment
	}
}

type pidController struct {
	config pidConfig

	integralError float64
	lastError     float64
	lastTime      time.Time
	initialized   bool
}

func newPIDController(config pidConfig) *pidController {
	return &pidController{config: config}
}

func (p *pidController) compute(setpoint, measurement, dt time.Duration) float64 {
	if !p.initialized {
		p.lastTime = time.Now()
		p.initialized = true
		return 0.0
	}

	// Normalized error: (actual - target) / target
	controlError := float64(measurement-setpoint) / float64(setpoint)

	// Calculate time delta in seconds
	dtSec := dt.Seconds()
	if dtSec <= 0 {
		dtSec = 1.0 // Default to 1s if invalid
	}

	// Proportional term
	proportional := p.config.kp * controlError

	// Integral term with anti-windup
	p.integralError += controlError * dtSec
	p.integralError = math.Max(p.config.minIntegral,
		math.Min(p.config.maxIntegral, p.integralError))
	integral := p.config.ki * p.integralError

	// Derivative term
	derivative := 0.0
	if dtSec > 0 {
		derivative = p.config.kd * (controlError - p.lastError) / dtSec
	}

	p.lastError = controlError

	// Compute output
	output := proportional + integral + derivative

	// Clamp output
	output = math.Max(p.config.minOutput,
		math.Min(p.config.maxOutput, output))

	return output
}

func (p *pidController) reset() {
	p.integralError = 0.0
	p.lastError = 0.0
	p.initialized = false
}
