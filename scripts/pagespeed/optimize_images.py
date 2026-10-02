"""Rebuild the bundled image variants with Pillow 11.3+ (AVIF support)."""
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parents[2] / "internal/ui/static/img"
for name, widths, encoding, options in [
    ("logo", (240, 480), "AVIF", {"quality": 50, "speed": 4}),
    ("doctor-capsule", (120, 240), "WEBP", {"quality": 85, "method": 6}),
]:
    with Image.open(root / f"{name}.png") as original:
        for width in widths:
            height = round(original.height * width / original.width)
            resized = original.resize((width, height), Image.Resampling.LANCZOS)
            resized.save(root / f"{name}-{width}.{encoding.lower()}", encoding, **options)
