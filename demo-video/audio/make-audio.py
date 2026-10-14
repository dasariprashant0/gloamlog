#!/usr/bin/env python3
"""make-audio.py - the demo's music bed and UI sounds, synthesised with the standard library only (no downloads, no packages).

Writes 44.1 kHz 16-bit stereo WAV files into public/audio/: bed.wav (a soft ambient pad, 110 s), pop.wav, click.wav, whoosh.wav,
chime.wav and preview-bed.wav (the first 15 s of the bed, to audition). Pure Python, so it takes a minute or two.
"""
import array
import math
import os
import random
import wave

SR = 44100
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "public", "audio")
os.makedirs(OUT, exist_ok=True)
random.seed(7)


def write_wav(name, left, right, peak_db):
    """Normalise to `peak_db` dBFS and write stereo 16-bit."""
    n = min(len(left), len(right))
    m = max(max(abs(x) for x in left[:n]), max(abs(x) for x in right[:n])) or 1.0
    g = (10 ** (peak_db / 20)) / m
    buf = array.array("h", bytes(4 * n))
    for i in range(n):
        buf[2 * i] = int(max(-1.0, min(1.0, left[i] * g)) * 32767)
        buf[2 * i + 1] = int(max(-1.0, min(1.0, right[i] * g)) * 32767)
    with wave.open(os.path.join(OUT, name), "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(SR)
        w.writeframes(buf.tobytes())
    jump = max(abs(buf[i] - buf[i - 2]) for i in range(2, 2 * n, 2)) / 32768
    print(f"{name}: {n / SR:.2f} s, peak {peak_db} dBFS, max sample jump {jump:.3f}")


def midi(m):
    return 440.0 * 2 ** ((m - 69) / 12)


# MARK: the pad
CHORDS = [  # C major, 8 s each, looped
    [48, 55, 59, 64, 67],  # Cmaj7
    [45, 52, 55, 60, 64],  # Am7
    [41, 48, 52, 57, 64],  # Fmaj7
    [43, 50, 55, 59, 62],  # G(add9)
    [40, 47, 52, 55, 62],  # Em7
    [45, 52, 57, 60, 64],  # Am7
    [38, 45, 50, 53, 60],  # Dm7
    [43, 50, 55, 60, 62],  # Gsus4 -> G colour
]
CHORD_S, BED_S, ATTACK, RELEASE = 8.0, 110.0, 2.5, 3.0


def note(freq, seconds, amp):
    """Three detuned sines plus a quiet octave, with attack and release; returns (left, right) lists."""
    n = int(seconds * SR)
    atk, rel = int(ATTACK * SR), int(RELEASE * SR)
    d = 0.002
    w0, w1, w2, w3 = (2 * math.pi * freq / SR, 2 * math.pi * freq * (1 + d) / SR, 2 * math.pi * freq * (1 - d) / SR, 2 * math.pi * freq * 2 / SR)
    p0 = random.random() * 6.28
    left = [0.0] * n
    right = [0.0] * n
    sin = math.sin
    for i in range(n):
        if i < atk:
            e = (i / atk) ** 2                     # gentle in
        elif i > n - rel:
            e = ((n - i) / rel) ** 1.5             # gentle out
        else:
            e = 1.0
        e *= amp * (1 + 0.15 * sin(2 * math.pi * 0.15 * i / SR + p0))      # slow breathing
        a, b, c, o = sin(w0 * i + p0), sin(w1 * i + p0), sin(w2 * i + p0), 0.3 * sin(w3 * i + p0)
        left[i] = e * (a + 0.7 * b + o)
        right[i] = e * (a + 0.7 * c + o)
    return left, right


def make_bed():
    total = int(BED_S * SR)
    chord_n = int(CHORD_S * SR)
    left = [0.0] * (total + int(RELEASE * SR) + 1)
    right = [0.0] * len(left)
    t = 0
    k = 0
    while t < total:
        for m in CHORDS[k % len(CHORDS)]:
            amp = 0.16 if m >= 48 else 0.12
            l, r = note(midi(m), CHORD_S + RELEASE, amp)
            for i in range(len(l)):
                j = t + i
                if j < len(left):
                    left[j] += l[i]
                    right[j] += r[i]
        t += chord_n
        k += 1
    left, right = left[:total], right[:total]
    # one-pole low-pass that opens a little within each chord, then a short comb "reverb"
    for ch in (left, right):
        y = 0.0
        for i in range(total):
            cut = 900 + 1500 * ((i % chord_n) / chord_n)
            a = 1 - math.exp(-2 * math.pi * cut / SR)
            y += a * (ch[i] - y)
            ch[i] = y
        d1, d2, fb = int(0.097 * SR), int(0.131 * SR), 0.35
        c1 = [0.0] * total
        c2 = [0.0] * total
        for i in range(total):
            c1[i] = ch[i] + (fb * c1[i - d1] if i >= d1 else 0.0)
            c2[i] = ch[i] + (fb * c2[i - d2] if i >= d2 else 0.0)
            ch[i] = ch[i] + 0.3 * 0.5 * (c1[i] + c2[i])
    fi, fo = int(3 * SR), int(4 * SR)
    for i in range(fi):
        left[i] *= i / fi
        right[i] *= i / fi
    for i in range(fo):
        g = i / fo
        left[total - 1 - i] *= g
        right[total - 1 - i] *= g
    write_wav("bed.wav", left, right, -6)
    write_wav("preview-bed.wav", left[: 15 * SR], right[: 15 * SR], -6)


# MARK: small sounds
def env(n, attack, decay):
    out = []
    for i in range(n):
        a = min(1.0, i / max(1, attack))
        d = math.exp(-i / decay) if decay else 1.0
        out.append(a * d)
    return out


def pop():
    n = int(0.25 * SR)
    e = env(n, int(0.002 * SR), 0.05 * SR)
    ph, out = 0.0, []
    for i in range(n):
        f = 520 - 200 * min(1.0, i / (0.06 * SR))
        ph += 2 * math.pi * f / SR
        s = math.sin(ph) * e[i]
        if i < int(0.005 * SR):
            s += 0.2 * (random.random() * 2 - 1) * (1 - i / (0.005 * SR))
        out.append(s)
    write_wav("pop.wav", out, out, -8)


def click():
    n = int(0.08 * SR)
    e1, e2 = env(n, 1, 0.006 * SR), env(n, 1, 0.02 * SR)
    lp, out = 0.0, []
    for i in range(n):
        noise = random.random() * 2 - 1
        lp += 0.25 * (noise - lp)
        out.append(0.8 * lp * e1[i] + 0.5 * math.sin(2 * math.pi * 180 * i / SR) * e2[i])
    write_wav("click.wav", out, out, -8)


def whoosh():
    n = int(0.45 * SR)
    lp = hp_prev = hp_out = 0.0
    out = []
    for i in range(n):
        t = i / n
        e = math.sin(math.pi * t) ** 1.5
        centre = 400 + 1200 * t
        a = 1 - math.exp(-2 * math.pi * centre / SR)
        noise = random.random() * 2 - 1
        lp += a * (noise - lp)                       # low-pass at the centre...
        hp_out = 0.98 * (hp_out + lp - hp_prev)      # ...then a high-pass, so it is a moving band
        hp_prev = lp
        out.append(hp_out * e)
    write_wav("whoosh.wav", out, out, -10)


def chime():
    n = int(0.9 * SR)
    out = [0.0] * n
    for start, f in ((0.0, 659.25), (0.25, 783.99)):
        s0 = int(start * SR)
        for i in range(n - s0):
            d = math.exp(-i / (0.5 * SR))
            t = i / SR
            out[s0 + i] += d * (math.sin(2 * math.pi * f * t) + 0.3 * math.sin(2 * math.pi * 2 * f * t) + 0.1 * math.sin(2 * math.pi * 3 * f * t))
    write_wav("chime.wav", out, out, -10)


if __name__ == "__main__":
    pop()
    click()
    whoosh()
    chime()
    make_bed()
    print("done:", OUT)
