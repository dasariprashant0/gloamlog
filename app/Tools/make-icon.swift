// Usage: make-icon <out.png> [full|simple] [pixels] [concept]   -- draws the Gloamlog icon (1024 canvas, 824 tile).
// concept: horizon (default) | moonpage | nib.  "simple" thickens the fine details (for pixel sizes <= 64).
// Pure CoreGraphics, no assets, deterministic.
import Foundation
import CoreGraphics
import ImageIO

let args = CommandLine.arguments
guard args.count >= 2 else { print("usage: make-icon out.png [full|simple] [pixels] [concept]"); exit(1) }
let simple = args.count > 2 && args[2] == "simple"
let px = args.count > 3 ? Int(args[3]) ?? 1024 : 1024
let concept = args.count > 4 ? args[4] : "horizon"
let S: CGFloat = 1024

func col(_ hex: UInt32, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: CGFloat((hex >> 16) & 255) / 255, green: CGFloat((hex >> 8) & 255) / 255,
            blue: CGFloat(hex & 255) / 255, alpha: a)
}
let cs = CGColorSpace(name: CGColorSpace.sRGB)!
let ctx = CGContext(data: nil, width: 1024, height: 1024, bitsPerComponent: 8, bytesPerRow: 0,
                    space: cs, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
ctx.setAllowsAntialiasing(true); ctx.interpolationQuality = .high

// All coordinates below are top-left origin (CG is bottom-left, so Y() flips).
func Y(_ y: CGFloat) -> CGFloat { S - y }
func P(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: x, y: Y(y)) }
func rr(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat, _ r: CGFloat) -> CGPath {
    CGPath(roundedRect: CGRect(x: x, y: S - y - h, width: w, height: h), cornerWidth: r, cornerHeight: r, transform: nil)
}
func grad(_ stops: [(CGFloat, UInt32, CGFloat)]) -> CGGradient {
    CGGradient(colorsSpace: cs, colors: stops.map { col($0.1, $0.2) } as CFArray, locations: stops.map { $0.0 })!
}
func linear(_ g: CGGradient, _ y0: CGFloat, _ y1: CGFloat) {
    ctx.drawLinearGradient(g, start: P(0, y0), end: P(0, y1), options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])
}
func radial(_ g: CGGradient, _ cx: CGFloat, _ cy: CGFloat, _ r: CGFloat) {
    ctx.drawRadialGradient(g, startCenter: P(cx, cy), startRadius: 0, endCenter: P(cx, cy), endRadius: r, options: [])
}
func fill(_ p: CGPath, _ c: CGColor) { ctx.addPath(p); ctx.setFillColor(c); ctx.fillPath() }

// Continuous-corner squircle (superellipse, n = 4.6 approximates Apple's tile).
func squircle(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat, n: CGFloat = 4.6) -> CGPath {
    let p = CGMutablePath(); let cx = x + w / 2, cy = y + h / 2, steps = 360
    for i in 0...steps {
        let t = CGFloat(i) / CGFloat(steps) * 2 * .pi
        let c = cos(t), s = sin(t)
        let px = cx + (w / 2) * (c < 0 ? -1 : 1) * pow(abs(c), 2 / n)
        let py = cy + (h / 2) * (s < 0 ? -1 : 1) * pow(abs(s), 2 / n)
        i == 0 ? p.move(to: P(px, py)) : p.addLine(to: P(px, py))
    }
    p.closeSubpath(); return p
}

// Shared palette: app accent family (Theme.swift: 0E7A5F / 45C29C) plus a warm dusk glow.
let deep = 0x0B2B26 as UInt32, green = 0x1B4A3E as UInt32, teal = 0x2A7A66 as UInt32
let amber = 0xF2A65A as UInt32, cream = 0xFFE6B8 as UInt32

let tile = squircle(100, 100, 824, 824)
ctx.saveGState()
ctx.setShadow(offset: CGSize(width: 0, height: -12), blur: 26, color: col(0, 0.30))
fill(tile, col(deep)); ctx.restoreGState()

ctx.saveGState(); ctx.addPath(tile); ctx.clip()

switch concept {
case "moonpage":
    // Dusk tile, a cream page that fades into the horizon, crescent moon above its lines.
    linear(grad([(0, deep, 1), (0.55, green, 1), (1, 0x3B6F5C, 1)]), 100, 924)
    radial(grad([(0, amber, 0.55), (1, amber, 0)]), 512, 930, 520)
    ctx.saveGState(); ctx.addPath(rr(282, 196, 460, 640, 40)); ctx.clip()
    linear(grad([(0, 0xFBF7EC, 1), (0.6, 0xF3EBD8, 1), (1, 0xE9B77A, 1)]), 196, 836)
    ctx.restoreGState()
    // crescent: disc minus offset disc
    ctx.saveGState()
    ctx.addRect(CGRect(x: 0, y: 0, width: S, height: S))
    ctx.addEllipse(in: CGRect(x: 596 - 64, y: Y(308) - 64, width: 128, height: 128))
    ctx.clip(using: .evenOdd)
    ctx.addEllipse(in: CGRect(x: 562 - 74, y: Y(330) - 74, width: 148, height: 148))
    ctx.setFillColor(col(0xE08A3C)); ctx.fillPath()
    ctx.restoreGState()
    let lw: [CGFloat] = [340, 340, 270, 190]
    for (i, y) in [(0, 470.0), (1, 548), (2, 626), (3, 704)] as [(Int, CGFloat)] {
        fill(rr(332, y, lw[i], simple ? 40 : 30, 15), col(0x1B4A3E, [0.55, 0.4, 0.28, 0.16][i]))
    }
case "nib":
    // Pen nib glowing against a low sun.
    linear(grad([(0, deep, 1), (0.5, green, 1), (1, 0x2F6F5B, 1)]), 100, 924)
    radial(grad([(0, cream, 0.95), (0.25, amber, 0.55), (1, amber, 0)]), 512, 640, 470)
    let nib = CGMutablePath()
    nib.move(to: P(512, 800))
    nib.addCurve(to: P(372, 520), control1: P(470, 740), control2: P(372, 640))
    nib.addCurve(to: P(448, 252), control1: P(372, 420), control2: P(448, 380))
    nib.addLine(to: P(576, 252))
    nib.addCurve(to: P(652, 520), control1: P(576, 380), control2: P(652, 420))
    nib.addCurve(to: P(512, 800), control1: P(652, 640), control2: P(554, 740))
    nib.closeSubpath()
    ctx.saveGState(); ctx.setShadow(offset: CGSize(width: 0, height: -10), blur: 24, color: col(0, 0.4))
    ctx.addPath(nib); ctx.setFillColor(col(0xF7ECD6)); ctx.fillPath(); ctx.restoreGState()
    ctx.saveGState(); ctx.addPath(nib); ctx.clip()
    linear(grad([(0, 0xFFF6E2, 1), (1, 0xE9B77A, 1)]), 252, 800); ctx.restoreGState()
    ctx.setStrokeColor(col(deep)); ctx.setLineCap(.round); ctx.setLineWidth(simple ? 26 : 18)
    ctx.move(to: P(512, 560)); ctx.addLine(to: P(512, 780)); ctx.strokePath()
    ctx.setFillColor(col(deep)); ctx.fillEllipse(in: CGRect(x: 512 - 30, y: Y(520) - 30, width: 60, height: 60))
default: // "horizon"
    // Dusk sky into a warm horizon; a half sun rests on it; below, its reflection is ruled journal lines that fade.
    let horizon: CGFloat = 590
    linear(grad([(0, deep, 1), (0.42, green, 1), (0.72, 0x2F7F6B, 1), (0.9, 0xC9A579, 1), (1, 0xF3B77C, 1)]), 100, horizon)
    radial(grad([(0, amber, 0.45), (1, amber, 0)]), 512, horizon, 300)
    // ground: dark band below the horizon
    ctx.saveGState(); ctx.clip(to: CGRect(x: 0, y: 0, width: S, height: Y(horizon)))
    linear(grad([(0, 0x143D34, 1), (1, 0x08201B, 1)]), horizon, 924); ctx.restoreGState()
    // sun
    ctx.saveGState(); ctx.clip(to: CGRect(x: 0, y: Y(horizon), width: S, height: horizon))
    let sunR: CGFloat = 172
    ctx.addEllipse(in: CGRect(x: 512 - sunR, y: Y(horizon) - sunR, width: sunR * 2, height: sunR * 2)); ctx.clip()
    linear(grad([(0, cream, 1), (1, amber, 1)]), horizon - sunR, horizon); ctx.restoreGState()
    // lines = reflection, fading and narrowing
    let ws: [CGFloat] = [380, 290, 200], al: [CGFloat] = [0.95, 0.62, 0.34]
    let lh: CGFloat = simple ? 44 : 34
    for i in 0..<3 {
        let y = horizon + 56 + CGFloat(i) * (simple ? 84 : 80)
        fill(rr(512 - ws[i] / 2, y, ws[i], lh, lh / 2), col(0xF4C27F, al[i]))
    }
}
ctx.restoreGState()

// Top rim light: 2.5px inner stroke that fades out by mid-height.
ctx.saveGState(); ctx.addPath(tile); ctx.clip()
ctx.addPath(squircle(101.5, 101.5, 821, 821)); ctx.setLineWidth(2.5); ctx.replacePathWithStrokedPath(); ctx.clip()
linear(grad([(0, 0xFFFFFF, 0.22), (1, 0xFFFFFF, 0)]), 100, 520)
ctx.restoreGState()

let url = URL(fileURLWithPath: args[1]) as CFURL
let dest = CGImageDestinationCreateWithURL(url, "public.png" as CFString, 1, nil)!
var out = ctx.makeImage()!
if px != 1024 { // downscale with high-quality interpolation
    let c2 = CGContext(data: nil, width: px, height: px, bitsPerComponent: 8, bytesPerRow: 0,
                       space: cs, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    c2.interpolationQuality = .high
    c2.draw(out, in: CGRect(x: 0, y: 0, width: px, height: px))
    out = c2.makeImage()!
}
CGImageDestinationAddImage(dest, out, nil)
guard CGImageDestinationFinalize(dest) else { print("write failed"); exit(1) }
