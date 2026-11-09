package budget

import "errors"

type DailyConfig struct {
	Margin float64
}

func loadDailyConfig() (DailyConfig, error) {
	return DailyConfig{}, errors.New("not implemented")
}
