# SPDX-License-Identifier: GPL-2.0-or-later
import schemdraw
import schemdraw.elements as elm

import os
base = os.path.join(os.path.dirname(os.path.abspath(__file__)), "")

def legs(d, q, bl="left"):
    d.add(elm.Label().at(q.collector).label("C", loc=bl, ofst=0.15, fontsize=10))
    d.add(elm.Label().at(q.emitter).label("E", loc=bl, ofst=0.15, fontsize=10))
    d.add(elm.Label().at(q.base).label("B", loc="top", ofst=0.15, fontsize=10))

# Figure A: PNP on the jumper pins (the backpack's transistor stays on).
with schemdraw.Drawing(show=False) as d:
    d.config(unit=2.2, fontsize=11, lw=1.6)
    pca = d.add(elm.Dot(open=True).label("PCA9685\nchannel 14 PWM pin\n(15 for seat 2)", loc="left", ofst=0.3))
    d.add(elm.Line().right(0.8))
    d.add(elm.Resistor().right().label("1 kΩ", loc="bottom"))
    q = d.add(elm.BjtPnp(circle=True).anchor("base")
              .label("Q1  PNP\n2N3906 / S8550 / BC557", loc="right", ofst=0.8))
    d.add(elm.Label().at(q.base).label("B", loc="top", ofst=0.15, fontsize=10))
    d.add(elm.Label().at(q.emitter).label("E", loc="right", ofst=0.15, fontsize=10))
    d.add(elm.Label().at(q.collector).label("C", loc="right", ofst=0.15, fontsize=10))
    d.add(elm.Line().at(q.emitter).up(0.8))
    d.add(elm.Dot(open=False).label("jumper pin, VCC side", loc="left", ofst=0.3))
    d.add(elm.Line().up(0.5))
    d.add(elm.Vdd().label("5 V (backpack VCC)", loc="top"))
    d.add(elm.Line().at(q.collector).down(0.8))
    d.add(elm.Dot(open=False).label("jumper pin, LCD side", loc="left", ofst=0.3))
    d.add(elm.Line().down(0.8))
    d.add(elm.Line().right(1.6))
    rled = d.add(elm.Resistor().right().label("LCD's own\nseries R", loc="bottom"))
    led = d.add(elm.LED().right().label("backlight LED", loc="bottom"))
    d.add(elm.Line().right(1.2))
    k = d.add(elm.Dot(open=False).label("pin 16 (K)", loc="top", ofst=0.3))
    d.add(elm.EncircleBox([rled, led], padx=0.5, pady=0.9).linestyle("--").color("gray")
          .label("1602 LCD module", loc="bottom", color="gray"))
    d.add(elm.Line().at(k.center).down(1.5))
    q0 = d.add(elm.BjtNpn(circle=True).anchor("collector").right()
               .label("backpack's own transistor\n(P3 held HIGH: stays ON)", loc="right", ofst=0.5))
    d.add(elm.Line().at(q0.emitter).down(0.6))
    d.add(elm.Ground())
    d.add(elm.Line().at(q0.base).left(0.8).label("P3", loc="left"))
    d.add(elm.Label().at((pca.center[0] - 1.0, q0.emitter[1] - 2.2)).label(
        "Jumper REMOVED; its two pins take Dupont wires to E and C. Nothing soldered to the backpack.\n"
        "PCA pin LOW = backlight on, so settings: \"backlights\": [14, 15], \"backlightsInvert\": true.\n"
        "The PCA9685 must share the backpack's rail (this cabinet: the 5 V chain), or the PNP never fully turns off.",
        fontsize=9, halign="left"))
    d.save(base + "backlight-pwm-jumper.svg")
    d.save(base + "backlight-pwm-jumper.png", dpi=150)

# Figure B: NPN on the LCD cathode pin (the backpack's transistor held off).
with schemdraw.Drawing(show=False) as d:
    d.config(unit=2.2, fontsize=11, lw=1.6)
    d.add(elm.Vdd().label("5 V\n(backpack VCC)", loc="left"))
    d.add(elm.Line().right(0.6))
    hdr = d.add(elm.Header(rows=1, cols=2, pinsleft=[], pinsright=[]).anchor("pin1")
                .label("LED jumper (leave IN)", loc="bottom", ofst=0.5))
    d.add(elm.Jumper().at(hdr.pin1))
    d.add(elm.Line().at(hdr.pin2).right(1.4))
    rled = d.add(elm.Resistor().right().label("LCD's own\nseries R", loc="top"))
    led = d.add(elm.LED().right().label("backlight LED", loc="top"))
    d.add(elm.Line().right(1.8))
    k = d.add(elm.Dot(open=False).label("LCD header pin 16 (K)", loc="top", ofst=0.3))
    d.add(elm.EncircleBox([rled, led], padx=0.5, pady=0.9).linestyle("--").color("gray")
          .label("1602 LCD module", loc="top", color="gray"))
    d.add(elm.Line().at(k.center).down(1.2))
    q = d.add(elm.BjtNpn(circle=True).anchor("collector").right()
              .label("Q1  NPN\n2N2222A / S8050 / BC337", loc="right", ofst=0.6))
    d.add(elm.Line().at(q.emitter).down(0.6))
    d.add(elm.Ground())
    legs(d, q)
    d.add(elm.Resistor().at(q.base).left().label("1 kΩ", loc="top"))
    d.add(elm.Line().left(1.0))
    pca = d.add(elm.Dot(open=True).label("PCA9685\nchannel 14 PWM pin\n(15 for seat 2)", loc="left", ofst=0.3))
    d.add(elm.Line().at(k.center).right(7.5))
    d.add(elm.Line().down(1.2))
    q0 = d.add(elm.BjtNpn(circle=True).anchor("collector").right().color("gray").linestyle("--")
               .label("backpack's own transistor\n(P3 held LOW: stays OFF)", loc="right", ofst=0.5, color="gray"))
    d.add(elm.Line().at(q0.emitter).down(0.6).color("gray").linestyle("--"))
    d.add(elm.Ground().color("gray"))
    d.add(elm.Line().at(q0.base).left(0.8).color("gray").linestyle("--").label("P3", loc="left", color="gray"))
    d.add(elm.Label().at((pca.center[0], q.emitter[1] - 2.4)).label(
        "One wire soldered to the top of LCD header pin 16; the jumper stays in.\n"
        "PCA pin HIGH = backlight on: settings \"backlights\": [14, 15], \"backlightsBackpackLow\": true.\n"
        "Works on any rail.",
        fontsize=9, halign="left"))
    d.save(base + "backlight-pwm-cathode.svg")
    d.save(base + "backlight-pwm-cathode.png", dpi=150)
print("ok")
