// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"os"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

type wifiConnectScene struct {
	a *app
}

func (s *wifiConnectScene) Init() error {
	border := 2
	qr, err := element.NewWifiQRCode(os.Getenv("WIFI_SSID"), os.Getenv("WIFI_PASS"), element.QRCodeOptions{
		Options: element.Options{OffsetX: 32, OffsetY: 32, Align: element.Center},
		Border:  &border,
	})
	if err != nil {
		return err
	}
	s.a.Ctx.AddLayer(modeWifiConnect, []display.Element{qr}, 0)
	return nil
}

func (s *wifiConnectScene) Enter() {
	s.a.Ctx.SetButtonLights(input.BothSeats, hardware.ButtonLights{"joyb": true}, true)
}

func (s *wifiConnectScene) HandleInput(clock.FrameTime, input.Event) bool { return false }

func (s *wifiConnectScene) Update(clock.FrameTime) {}

func (s *wifiConnectScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *wifiConnectScene) Teardown() {}
