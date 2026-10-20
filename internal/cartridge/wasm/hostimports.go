// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// hostImport's fn takes a context, optionally the api.Module, then the wire
// arguments with the wire's signedness, which hostimports_test.go checks
// against sdk/cartlib.
type hostImport struct {
	name string
	fn   any
}

func instantiateHostModule(ctx context.Context, rt wazero.Runtime, module string, imports []hostImport) (api.Module, error) {
	b := rt.NewHostModuleBuilder(module)
	for _, imp := range imports {
		b.NewFunctionBuilder().WithFunc(imp.fn).Export(imp.name)
	}
	return b.Instantiate(ctx)
}

// arkadeImports is the "arkade" module: each import name and the role method
// that serves it. There is no host_draw_text: text rasterizes guest-side, so
// both hosts blit identical bytes.
func (br *bridge) arkadeImports() []hostImport {
	draw, sheets := br.surface, sheetImports{br.sheets}
	sound, store, scores := br.speaker, br.storage, br.scores
	ctl, lcd, session := br.control, br.lcd, br.session
	channels, lamps := channelImports{br.channels}, lampImports{br.lights}
	return []hostImport{
		{"host_blit", draw.blit},
		{"host_draw_pixel", draw.drawPixel},
		{"host_fill_rect", draw.fillRect},
		{"host_load_sprite", draw.loadSprite},
		{"host_draw_sprite", draw.drawSprite},

		{"host_load_sheet", sheets.load},
		{"host_sheet_cel", sheets.celNamed},
		{"host_cel_info", sheets.info},
		{"host_cel_read", sheets.read},
		{"host_draw_cel", sheets.draw},

		{"host_load_sound", sound.load},
		{"host_play_sound", sound.play},
		{"host_stop_sound", sound.stop},
		{"host_stop_all_sounds", sound.stopAll},

		{"host_storage_read", store.read},
		{"host_storage_write", store.write},

		{"host_score_submit", scores.submit},
		{"host_score_read", scores.read},
		{"host_score_count", scores.count},
		{"host_score_qualifies", scores.qualifies},
		{"host_score_rank", scores.rank},
		{"host_recent_name_get", scores.recentName},
		{"host_recent_name_at", scores.recentNameAt},
		{"host_recent_name_note", scores.noteRecentName},

		{"host_push_player_strategy", ctl.pushStrategy},
		{"host_pop_player_strategy", ctl.popStrategy},
		{"host_lock_player_strategy", ctl.lockStrategy},
		{"host_unlock_player_strategy", ctl.unlockStrategy},
		{"host_switch_player", ctl.switchPlayer},
		{"host_lock_player_seat", ctl.lockSeat},
		{"host_set_raw_directions", ctl.setRawDirections},
		{"host_reject_key", ctl.rejectKey},
		{"host_exit", ctl.exit},

		{"host_set_text", lcd.setText},
		{"host_define_lcd_glyph", lcd.defineGlyph},

		{"host_set_button_lights", lamps.setButtonLights},
		{"host_set_lights", lamps.setLights},
		{"host_set_lamp_levels", lamps.setLampLevels},
		{"host_lights_play", lamps.playSequence},
		{"host_lights_stop", lamps.stopSequence},

		{"host_input_bind", channels.bind},
		{"host_input_button", channels.button},
		{"host_input_axis", channels.axis},
		{"host_input_delta", channels.delta},
		{"host_input_blob", channels.blob},
		{"host_output_bind", channels.outputBind},
		{"host_output_set", channels.outputSet},

		{"host_now_micros", session.nowMicros},
		{"host_random_u64", session.randomU64},
		{"host_log", session.log},
	}
}
