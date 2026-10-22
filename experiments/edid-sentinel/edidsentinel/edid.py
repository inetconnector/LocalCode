"""EDID corpus and structure-aware mutations; offline testing only."""
from __future__ import annotations
import random

HEADER = bytes.fromhex("00ffffffffffff00")
BOUNDARY = (0, 1, 2, 3, 15, 16, 31, 63, 127, 128, 254, 255)
DIM_BOUNDARY = (0, 1, 2, 255, 256, 4094, 4095)

def fix_checksums(data: bytes) -> bytes:
    b = bytearray(data)
    for start in range(0, len(b)//128*128, 128):
        b[start+127] = (-sum(b[start:start+127])) & 255
    return bytes(b)

def make_seed(extension: str = "base") -> bytes:
    b = bytearray(128)
    b[:8] = HEADER
    b[8:10] = bytes.fromhex("1234")
    b[10:12] = bytes.fromhex("7856")
    b[16:18] = bytes.fromhex("0124")
    b[18:20] = bytes.fromhex("0104")
    b[20:25] = bytes([0x80, 52, 30, 120, 0x0a])
    b[35:38] = bytes([0x21, 0x08, 0])
    for p in range(38, 54, 2):
        b[p:p+2] = bytes.fromhex("0101")
    b[54:72] = bytes.fromhex("023a801871382d40582c4500341d0000001e")
    b[72:90] = bytes.fromhex("000000fc0053454e54494e454c0a20202020")
    b[126] = 0 if extension == "base" else 1
    if extension == "base":
        return fix_checksums(b)
    ext = bytearray(128)
    if extension == "cta":
        ext[:7] = bytes([0x02, 0x03, 0x07, 0, 0x41, 0x10, 0])
    elif extension == "displayid":
        ext[:6] = bytes([0x70, 0x12, 0, 0, 0, 0])
    else:
        raise ValueError("unknown extension: " + extension)
    return fix_checksums(b+ext)

def mutate(data: bytes, rng: random.Random, max_bytes: int = 4096) -> tuple[bytes,list[str]]:
    b = bytearray(data or make_seed())
    applied = []
    ops = (_boundary_byte, _bit_flip, _dtd_dimensions, _dtd_pixel_clock,
           _extension_count, _extension_lengths, _truncate,
           _duplicate_or_append, _clobber_header, _dtd_blanking, _cross_field)
    for _ in range(rng.randint(1, 3)):
        op = rng.choice(ops)
        op(b, rng)
        applied.append(op.__name__.removeprefix("_"))
        if len(b) > max_bytes:
            del b[max_bytes:]
    if len(b) and rng.random() < .70:
        b[:] = fix_checksums(b)
        applied.append("fix_checksums")
    return bytes(b), applied

def _boundary_byte(b, rng):
    if b:
        b[rng.randrange(len(b))] = rng.choice(BOUNDARY)

def _bit_flip(b, rng):
    if b:
        b[rng.randrange(len(b))] ^= 1 << rng.randrange(8)

def _dtd_dimensions(b, rng):
    if len(b) < 72:
        return
    base = 54+18*rng.randrange(4)
    if base+8 >= len(b):
        return
    dim = rng.choice(DIM_BOUNDARY)
    if rng.randrange(2):
        b[base+2] = dim & 255
        b[base+4] = ((dim >> 8) << 4) | (b[base+4]&15)
    else:
        b[base+5] = dim & 255
        b[base+7] = ((dim >> 8) << 4) | (b[base+7]&15)

def _dtd_pixel_clock(b, rng):
    if len(b) >= 72:
        base = 54+18*rng.randrange(4)
        if base+1 < len(b):
            b[base:base+2] = rng.choice((0,1,0x7fff,0xffff)).to_bytes(2,"little")

def _dtd_blanking(b, rng):
    if len(b) >= 72:
        base = 54+18*rng.randrange(4)
        if base+7 < len(b):
            value = rng.choice(DIM_BOUNDARY)
            off = rng.choice((3,6))
            msb = 4 if off == 3 else 7
            b[base+off] = value & 255
            b[base+msb] = (b[base+msb]&0xf0) | (value >> 8)

def _extension_count(b, rng):
    if len(b) >= 127:
        b[126] = rng.choice((0,1,2,3,127,254,255))

def _extension_lengths(b, rng):
    if len(b) < 256:
        return
    ext = 128*rng.randrange(1,len(b)//128)
    if ext+6 < len(b):
        b[ext] = rng.choice((2,0x70,0,0xff))
        b[ext+rng.choice((2,3,4))] = rng.choice(BOUNDARY)
        if b[ext] == 2 and ext+5 < len(b):
            b[ext+4] = rng.choice((0x1f,0x3f,0x5f,0x7f,0xff))

def _truncate(b, rng):
    if len(b)>1:
        choices = [v for v in (0,1,7,8,18,54,126,127,128,129,255,256) if v<len(b)]
        del b[rng.choice(choices):]

def _duplicate_or_append(b,rng):
    if b:
        amount=rng.choice((1,2,16,127,128,255))
        off=rng.randrange(len(b))
        b.extend(b[off:off+amount] or b"\x00")

def _clobber_header(b,rng):
    if len(b)>=8:
        b[:8]=HEADER if rng.random()<.5 else bytes(rng.choice(BOUNDARY) for _ in range(8))

def _cross_field(b,rng):
    if len(b)>=128:
        _dtd_dimensions(b,rng)
        _dtd_blanking(b,rng)
        _extension_count(b,rng)
        if len(b)>=256:
            _extension_lengths(b,rng)
