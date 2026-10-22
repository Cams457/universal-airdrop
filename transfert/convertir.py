"""Conversions optionnelles après réception (désactivées par défaut) : HEIC -> JPEG, HEVC -> H.264.

L'original est toujours conservé ; la conversion ajoute une copie à côté.
"""

import shutil
import subprocess
from pathlib import Path


def heic_vers_jpeg(chemin: Path) -> Path | None:
    try:
        import pillow_heif
        from PIL import Image
    except ImportError:
        return None
    pillow_heif.register_heif_opener()
    sortie = chemin.with_suffix(".jpg")
    if sortie.exists():
        return None
    with Image.open(chemin) as img:
        img.convert("RGB").save(sortie, "JPEG", quality=92)
    return sortie


def est_hevc(chemin: Path) -> bool:
    if not shutil.which("ffprobe"):
        return False
    r = subprocess.run(["ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries",
                        "stream=codec_name", "-of", "csv=p=0", str(chemin)],
                       capture_output=True, text=True, timeout=30)
    return r.stdout.strip() == "hevc"


def hevc_vers_h264(chemin: Path) -> Path | None:
    if not shutil.which("ffmpeg") or not est_hevc(chemin):
        return None
    sortie = chemin.with_name(chemin.stem + "_h264.mp4")
    subprocess.run(["ffmpeg", "-n", "-v", "error", "-i", str(chemin), "-c:v", "libx264", "-crf", "20",
                    "-preset", "fast", "-c:a", "aac", str(sortie)], timeout=3600)
    return sortie if sortie.exists() else None


def traiter(chemin: Path, convertir_heic: bool, convertir_hevc: bool) -> Path | None:
    ext = chemin.suffix.lower()
    if convertir_heic and ext in (".heic", ".heif"):
        return heic_vers_jpeg(chemin)
    if convertir_hevc and ext in (".mov", ".mp4", ".m4v"):
        return hevc_vers_h264(chemin)
    return None
