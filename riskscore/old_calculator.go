// internal/riskscore/calculator.go

package riskscore

import (
    "math"
)

// RiskScoreParams contains parameters for risk score calculation
type RiskScoreParams struct {
    BaseScore       float64 // base_score(E)
    ObjectCount     int     // n(E)
    RuleFrequency   float64 // f_rule(E)
    TimeInterval    float64 // T(E)
    Lambda          float64 // λ
    K               float64 // k
    C               float64 // c
    D               float64 // d
    MinScore        float64 // min_score
    ImportanceFactor float64 // importance_factor
    MaxScore        float64 // R_max
}

// CalculateRiskScore calculates the risk score for an event based on the formula
func CalculateRiskScore(params RiskScoreParams) float64 {
    // Step a: Reduce score if rule fires for many objects (high dispersion)
    // partial₁(E) = base_score(E) × exp(-λ × n(E))
    partial1 := params.BaseScore * math.Exp(-params.Lambda * float64(params.ObjectCount))

    // Step b: Reduce score if rule has regular alert frequency (noisy rule)
    // partial₂(E) = partial₁(E) / (1 + k × f_rule(E))
    partial2 := partial1 / (1 + params.K * params.RuleFrequency)

    // Step c: Increase score if object is alerted by different rules in short time
    // partial₃(E) = partial₂(E) + c / (1 + d / T(E))
    partial3 := partial2
    if params.TimeInterval > 0 {
        partial3 += params.C / (1 + params.D / params.TimeInterval)
    }

    // Step d: Ensure score doesn't drop below minimum threshold
    // partial₄(E) = max(partial₃(E), min_score)
    partial4 := math.Max(partial3, params.MinScore)

    // Apply importance factor
    score := partial4 * params.ImportanceFactor

    // Apply maximum cap
    return math.Min(score, params.MaxScore)
}

// CalculateSessionRiskScore calculates the total risk score for a session
func CalculateSessionRiskScore(eventScores []float64, importanceFactor, maxScore float64) float64 {
    // Sum all event scores in the session
    var totalScore float64
    for _, score := range eventScores {
        totalScore += score
    }

    // Apply importance factor
    totalScore *= importanceFactor

    // Apply maximum cap
    return math.Min(totalScore, maxScore)
}
