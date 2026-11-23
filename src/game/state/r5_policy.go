package state

// SafeGameplayPolicy is a launch/result interpretation, never a current SETUP
// preference. Historical films and pending proposals keep their recorded rules.
const SafeGameplayPolicy = "safe-gameplay-stepR5-v1"

func validGameplayPolicy(policy string) bool { return policy == "" || policy == SafeGameplayPolicy }
