// Usage: make-social <out.png> <AppIcon-1024.png>   -- draws the 1280x640 Open Graph / GitHub social preview for the site.
// Same house style as app/Tools/make-icon.swift: pure CoreGraphics (+ CoreText for the two lines of type), deterministic,
// no assets beyond the app icon it is handed. Palette is the icon's dusk sky and warm horizon.
//   swiftc -O site/tools/make-social.swift -o /tmp/make-social && /tmp/make-social site/assets/social.png app/Resources/AppIcon-1024.png
import Foundation
import CoreGraphics
import CoreText
import ImageIO

let args = CommandLine.arguments
guard args.count >= 3 else { print("usage: make-social out.png AppIcon-1024.png"); exit(1) }
let W: CGFloat = 1280, H: CGFloat = 640

func col(_ hex: UInt32, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: CGFloat((hex >> 16) & 255) / 255, green: CGFloat((hex >> 8) & 255) / 255,
            blue: CGFloat(hex & 255) / 255, alpha: a)
}
let cs = CGColorSpace(name: CGColorSpace.sRGB)!
let ctx = CGContext(data: nil, width: Int(W), height: Int(H), bitsPerComponent: 8, bytesPerRow: 0,
                    space: cs, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
ctx.setAllowsAntialiasing(true); ctx.interpolationQuality = .high

// All coordinates below are top-left origin (CG is bottom-left, so Y() flips).
func Y(_ y: CGFloat) -> CGFloat { H - y }
func P(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: x, y: Y(y)) }
func grad(_ stops: [(CGFloat, UInt32, CGFloat)]) -> CGGradient {
    CGGradient(colorsSpace: cs, colors: stops.map { col($0.1, $0.2) } as CFArray, locations: stops.map { $0.0 })!
}
func linear(_ g: CGGradient, _ y0: CGFloat, _ y1: CGFloat) {
    ctx.drawLinearGradient(g, start: P(0, y0), end: P(0, y1), options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])
}
func radial(_ g: CGGradient, _ cx: CGFloat, _ cy: CGFloat, _ r: CGFloat) {
    ctx.drawRadialGradient(g, startCenter: P(cx, cy), startRadius: 0, endCenter: P(cx, cy), endRadius: r, options: [])
}

// Shared palette (app/Tools/make-icon.swift): app accent family plus the warm dusk glow.
let deep = 0x0B2B26 as UInt32, green = 0x1B4A3E as UInt32
let amber = 0xF2A65A as UInt32, cream = 0xFFE6B8 as UInt32

// Dusk sky that warms toward a low horizon, dark ground below it, a soft sun glow sitting on the line.
let horizon: CGFloat = 572
linear(grad([(0, deep, 1), (0.5, green, 1), (0.8, 0x2F7F6B, 1), (0.94, 0xB89A6E, 1), (1, 0xF3B77C, 1)]), 0, horizon)
radial(grad([(0, amber, 0.40), (1, amber, 0)]), 840, horizon, 360)
ctx.saveGState(); ctx.clip(to: CGRect(x: 0, y: 0, width: W, height: Y(horizon)))
linear(grad([(0, 0x143D34, 1), (1, 0x08201B, 1)]), horizon, H); ctx.restoreGState()

// The app icon, as shipped (its own tile, shadow and horizon), on the left.
let src = CGImageSourceCreateWithURL(URL(fileURLWithPath: args[2]) as CFURL, nil)
guard let src, let icon = CGImageSourceCreateImageAtIndex(src, 0, nil) else { print("cannot read icon"); exit(1) }
let iconSize: CGFloat = 440                     // 1024 canvas scaled; the tile inside is 80% of it
ctx.draw(icon, in: CGRect(x: 88, y: Y(100 + iconSize), width: iconSize, height: iconSize))

// Type: the name and the one-sentence tagline, system font so it matches the app and the site.
func line(_ s: String, _ font: CTFont, _ color: UInt32, _ alpha: CGFloat, kern: CGFloat = 0) -> CTLine {
    let attrs: [CFString: Any] = [kCTFontAttributeName: font, kCTForegroundColorAttributeName: col(color, alpha),
                                  kCTKernAttributeName: kern]
    return CTLineCreateWithAttributedString(CFAttributedStringCreate(nil, s as CFString, attrs as CFDictionary))
}
func draw(_ l: CTLine, x: CGFloat, baseline: CGFloat) -> CGFloat {
    ctx.textMatrix = .identity
    ctx.textPosition = P(x, baseline)
    CTLineDraw(l, ctx)
    return CTLineGetTypographicBounds(l, nil, nil, nil)
}
let textX: CGFloat = 556, maxW = W - textX - 72
let title = CTFontCreateUIFontForLanguage(.emphasizedSystem, 108, nil)!
let body = CTFontCreateUIFontForLanguage(.system, 33, nil)!
var widths: [CGFloat] = []
widths.append(draw(line("Gloamlog", title, 0xFFFFFF, 1, kern: -2.5), x: textX, baseline: 284))
let tagline = ["A work journal for your Mac that can write",
               "itself from Claude Code and Git, while",
               "everything stays on your Mac."]
for (i, t) in tagline.enumerated() {
    widths.append(draw(line(t, body, cream, 0.92), x: textX, baseline: 352 + CGFloat(i) * 46))
}
guard let widest = widths.max(), widest <= maxW else { print("text overflows: \(widths) > \(maxW)"); exit(1) }

let url = URL(fileURLWithPath: args[1]) as CFURL
let dest = CGImageDestinationCreateWithURL(url, "public.png" as CFString, 1, nil)!
CGImageDestinationAddImage(dest, ctx.makeImage()!, nil)
guard CGImageDestinationFinalize(dest) else { print("write failed"); exit(1) }
