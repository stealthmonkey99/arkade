// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import "github.com/stealthmonkey99/arkade/internal/storage"

// LoadConfig reads <dataDir>/hardware/settings.json over DefaultConfig. Fields
// are pointers so an omitted one keeps its default.
func LoadConfig(dataDir string) Config {
	cfg := DefaultConfig()
	var settings struct {
		PCA9685 *struct {
			Enabled               *bool                    `json:"enabled"`
			Addr                  *uint16                  `json:"addr"`
			FrequencyHz           *int                     `json:"frequencyHz"`
			Invert                *bool                    `json:"invert"`
			Lamps                 *[2][lightsPerPlayer]int `json:"lamps"`
			Backlights            *[2]int                  `json:"backlights"`
			BacklightsInvert      *bool                    `json:"backlightsInvert"`
			BacklightsBackpackLow *bool                    `json:"backlightsBackpackLow"`
		} `json:"pca9685"`
		Fan *struct {
			ExpanderPins *[]int `json:"expanderPins"`
			PCAChannels  *[]int `json:"pcaChannels"`
			Invert       *bool  `json:"invert"`
		} `json:"fan"`
	}
	if err := storage.New(dataDir).Read("hardware", "", &settings); err != nil {
		return cfg
	}
	if f := settings.Fan; f != nil {
		if f.ExpanderPins != nil {
			cfg.Fan.ExpanderPins = *f.ExpanderPins
		}
		if f.PCAChannels != nil {
			cfg.Fan.PCAChannels = *f.PCAChannels
		}
		if f.Invert != nil {
			cfg.Fan.Invert = *f.Invert
		}
	}
	p := settings.PCA9685
	if p == nil {
		return cfg
	}
	if p.Enabled != nil {
		cfg.PCA9685.Enabled = p.Enabled
	}
	if p.Addr != nil {
		cfg.PCA9685.Addr = *p.Addr
	}
	if p.FrequencyHz != nil {
		cfg.PCA9685.FrequencyHz = *p.FrequencyHz
	}
	if p.Invert != nil {
		cfg.PCA9685.Invert = *p.Invert
	}
	if p.Lamps != nil {
		cfg.PCA9685.Lamps = *p.Lamps
	}
	if p.Backlights != nil {
		cfg.PCA9685.Backlights = *p.Backlights
	}
	if p.BacklightsInvert != nil {
		cfg.PCA9685.BacklightsInvert = *p.BacklightsInvert
	}
	if p.BacklightsBackpackLow != nil {
		cfg.PCA9685.BacklightsBackpackLow = *p.BacklightsBackpackLow
	}
	return cfg
}
