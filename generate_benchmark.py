#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
generate_benchmark_report.py

Génère automatiquement un rapport PDF complet (CPU, mémoire, hyperfine,
diagnostic) à partir des fichiers produits par benchmark.ps1 dans un dossier
de benchmarks : cpu-*.prof, memory-*.prof, gc-*.log, hyperfine-*.json,
latest.json, latest.md.

Conçu pour être appelé automatiquement à la fin de benchmark.ps1 :

    python generate_benchmark_report.py --dir "C:\\Users\\salou\\JeuDeLaVie\\benchmarks"

Ne code EN DUR aucune valeur de mesure : tout est relu depuis les fichiers
les plus récents du dossier, donc le rapport reste correct à chaque nouveau
run (même si le hotspot change après une optimisation).

Prérequis (une seule fois) :
    pip install reportlab matplotlib pillow
    Go doit être installé et "go" doit être dans le PATH (go tool pprof).
    Optionnel mais recommandé : Graphviz installé et "dot" dans le PATH,
    pour les graphes d'appel visuels (sinon le rapport les omet simplement).
"""

import argparse
import glob
import os
import re
import shutil
import subprocess
import sys
import tempfile
import json
from datetime import datetime, timezone

# ----------------------------------------------------------------------
# Dependency check with a friendly message (this runs unattended after
# benchmark.ps1, so failures need to be understandable without a debugger)
# ----------------------------------------------------------------------
try:
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
except ImportError:
    sys.exit("[generate_benchmark_report] matplotlib manquant. "
              "Installez-le avec : pip install matplotlib")

try:
    from reportlab.lib.pagesizes import A4
    from reportlab.lib.units import mm
    from reportlab.lib import colors
    from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
    from reportlab.platypus import (SimpleDocTemplate, Paragraph, Spacer, Image,
                                     Table, TableStyle, PageBreak, HRFlowable)
    from reportlab.lib.enums import TA_LEFT, TA_CENTER
    from reportlab.pdfbase import pdfmetrics
    from reportlab.pdfbase.ttfonts import TTFont
except ImportError:
    sys.exit("[generate_benchmark_report] reportlab manquant. "
              "Installez-le avec : pip install reportlab")


# ========================================================================
# 1. Localisation des fichiers les plus récents dans le dossier
# ========================================================================

def find_latest(directory, pattern):
    """Retourne le fichier le plus récent (mtime) correspondant au pattern,
    ou None s'il n'y en a aucun."""
    matches = glob.glob(os.path.join(directory, pattern))
    if not matches:
        return None
    return max(matches, key=os.path.getmtime)


def load_json(path):
    if not path or not os.path.exists(path):
        return None
    with open(path, "r", encoding="utf-8-sig") as f:
        return json.load(f)


def extract_notes_from_md(path):
    """Récupère la section '## Notes' de latest.md si elle existe, pour que
    les notes manuelles écrites par l'outil de benchmark soient reprises
    automatiquement dans le PDF."""
    if not path or not os.path.exists(path):
        return []
    with open(path, "r", encoding="utf-8-sig", errors="replace") as f:
        text = f.read()
    if "Ã" in text or "Â" in text:
        try:
            text = text.encode("cp1252").decode("utf-8")
        except UnicodeError:
            try:
                text = text.encode("latin-1").decode("utf-8")
            except UnicodeError:
                pass
    m = re.search(r"##\s*Notes\s*\n(.*?)(\n##|\Z)", text, re.DOTALL)
    if not m:
        return []
    lines = [l.strip(" -\t") for l in m.group(1).splitlines() if l.strip().startswith("-")]
    return lines


# ========================================================================
# 2. go tool pprof : extraction texte + graphes
# ========================================================================

def run_pprof_top(profile_path, extra_flag=None, nodecount=25):
    """Lance `go tool pprof -top` et retourne la sortie texte, ou None si
    go/le profil est indisponible."""
    if not profile_path or not shutil.which("go"):
        return None
    cmd = ["go", "tool", "pprof", "-top", f"-nodecount={nodecount}"]
    if extra_flag:
        cmd.append(extra_flag)
    cmd.append(profile_path)
    try:
        out = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        return out.stdout if out.returncode == 0 else None
    except Exception:
        return None


def run_pprof_png(profile_path, out_png, extra_flag=None, nodecount=15):
    """Génère un graphe d'appel PNG via graphviz. Retourne True si réussi."""
    dot_path = shutil.which("dot")
    if not dot_path and os.name == "nt":
        candidate = os.path.join(os.environ.get("ProgramFiles", "C:\\Program Files"), "Graphviz", "bin", "dot.exe")
        if os.path.isfile(candidate):
            dot_path = candidate
    if not profile_path or not shutil.which("go") or not dot_path:
        return False
    cmd = ["go", "tool", "pprof", "-png", f"-nodecount={nodecount}"]
    if extra_flag:
        cmd.append(extra_flag)
    cmd.append(profile_path)
    try:
        env = os.environ.copy()
        env["PATH"] = os.path.dirname(dot_path) + os.pathsep + env.get("PATH", "")
        with open(out_png, "wb") as f:
            result = subprocess.run(cmd, stdout=f, stderr=subprocess.PIPE, timeout=60, env=env)
        return result.returncode == 0 and os.path.getsize(out_png) > 0
    except Exception:
        return False


_VALUE_RE = re.compile(r"^([\d.]+)([a-zA-Z%]*)$")

def parse_value(token):
    """'26.63s' -> (26.63,'s'); '362.39MB' -> (362.39,'MB'); '98817' -> (98817.0,'')"""
    m = _VALUE_RE.match(token)
    if not m:
        return (0.0, "")
    return (float(m.group(1)), m.group(2))


_ROW_RE = re.compile(
    r"^\s*([\d.]+[a-zA-Z%]*)\s+([\d.]+)%\s+([\d.]+)%\s+([\d.]+[a-zA-Z%]*)\s+([\d.]+)%\s+(.+?)\s*$"
)
_TOTAL_RE = re.compile(r"of\s+([\d.]+[a-zA-Z]*)\s+total")
_TYPE_RE = re.compile(r"^Type:\s*(\S+)")


def parse_pprof_top(text):
    """Parse la sortie de `go tool pprof -top` en une structure exploitable :
    {'type': str, 'total_value': float, 'total_unit': str, 'rows': [...]}"""
    if not text:
        return None
    ptype = None
    total_value, total_unit = None, ""
    rows = []
    for line in text.splitlines():
        tm = _TYPE_RE.match(line)
        if tm:
            ptype = tm.group(1)
        totm = _TOTAL_RE.search(line)
        if totm and total_value is None:
            total_value, total_unit = parse_value(totm.group(1))
        rm = _ROW_RE.match(line)
        if rm:
            flat_str, flat_pct, sum_pct, cum_str, cum_pct, func = rm.groups()
            flat_val, flat_unit = parse_value(flat_str)
            cum_val, cum_unit = parse_value(cum_str)
            rows.append({
                "flat_str": flat_str, "flat_val": flat_val, "flat_unit": flat_unit,
                "flat_pct": float(flat_pct),
                "cum_str": cum_str, "cum_val": cum_val, "cum_unit": cum_unit,
                "cum_pct": float(cum_pct),
                "func": func.strip(),
            })
    if total_value is None and rows:
        # Repli : estimer le total à partir du plus grand cum%/flat% si absent
        total_value = max((r["cum_val"] * 100.0 / r["cum_pct"] for r in rows if r["cum_pct"] > 0), default=0.0)
    return {"type": ptype, "total_value": total_value or 0.0, "total_unit": total_unit, "rows": rows}


_GC_RE = re.compile(
    r"gc\s+(\d+)\s+@[^:]+:\s+([^ ]+)\+([^ ]+)\+([^ ]+)\s+ms clock,\s+([^ ]+)\+([^ ]+)\+([^ ]+)\s+ms cpu,\s+[^,]+,\s+([\d.]+)\s+MB goal"
)


def parse_gc_trace(path):
    """Parse les lignes gctrace produites par le runtime Go."""
    if not path or not os.path.exists(path):
        return None
    cycles = []
    with open(path, "rb") as stream:
        raw = stream.read()
    if raw.startswith((b"\xff\xfe", b"\xfe\xff")) or b"\x00" in raw[:200]:
        text = raw.decode("utf-16", errors="replace")
    else:
        text = raw.decode("utf-8", errors="replace")
    text = re.sub(r"\s+", " ", text)
    for match in _GC_RE.finditer(text):
        values = []
        for value in match.groups()[1:]:
            values.append(sum(float(part) for part in re.split(r"[+/]", value) if part))
        cycles.append({
            "number": int(match.group(1)),
            "pauseMs": sum(values[0:3]),
            "cpuMs": sum(values[3:6]),
            "goalMb": values[6],
        })
    if not cycles:
        return {"cycles": 0, "totalPauseMs": 0, "maxPauseMs": 0, "totalCpuMs": 0, "maxGoalMb": 0}
    return {
        "cycles": len(cycles),
        "totalPauseMs": sum(item["pauseMs"] for item in cycles),
        "maxPauseMs": max(item["pauseMs"] for item in cycles),
        "totalCpuMs": sum(item["cpuMs"] for item in cycles),
        "maxGoalMb": max(item["goalMb"] for item in cycles),
    }


# ========================================================================
# 3. Graphiques (générés à partir des données parsées, jamais codés en dur)
# ========================================================================

COLORS_HEX = ["#D64545", "#4C7DBF", "#3FA796", "#E8A33D", "#8A6FBF"]

def make_breakdown_chart(parsed, out_path, title, ylabel, top_n=4):
    """Barres : les top_n fonctions par flat_val, + un 'reste' pour combler
    jusqu'au total mesuré."""
    if not parsed or not parsed["rows"]:
        return False
    rows = sorted(parsed["rows"], key=lambda r: r["flat_val"], reverse=True)[:top_n]
    unit = rows[0]["flat_unit"] if rows else parsed["total_unit"]
    labels = [r["func"].split(".")[-1][:22] for r in rows]
    values = [r["flat_val"] for r in rows]
    total = parsed["total_value"] or sum(values)
    reste = max(total - sum(values), 0)
    if reste > 0:
        labels.append("reste")
        values.append(reste)
    colors_ = COLORS_HEX[:len(values) - (1 if reste > 0 else 0)] + (["#D9D9D9"] if reste > 0 else [])

    fig, ax = plt.subplots(figsize=(6.4, 4.2), dpi=200)
    bars = ax.bar(labels, values, color=colors_, edgecolor="white", linewidth=0.6)
    ymax = max(values) * 1.18 if values else 1
    for b, v in zip(bars, values):
        pct = (v / total * 100) if total else 0
        ax.text(b.get_x() + b.get_width() / 2, v + ymax * 0.015,
                 f"{v:.2f}{unit}\n({pct:.1f}%)", ha="center", va="bottom",
                 fontsize=8.5, fontweight="bold")
    ax.set_ylabel(ylabel)
    ax.set_title(title, fontsize=10.5)
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.set_ylim(0, ymax)
    plt.xticks(rotation=12, ha="right", fontsize=8.5)
    plt.tight_layout()
    plt.savefig(out_path)
    plt.close()
    return True


def make_complexity_illustration(out_path):
    """Schéma pédagogique générique (échelle arbitraire, non mesurée) —
    rappel visuel de pourquoi un scan complet par cellule/élément explose
    plus vite qu'une recherche bornée. Constant d'un rapport à l'autre."""
    import numpy as np
    n = np.linspace(1, 10, 100)
    fig, ax = plt.subplots(figsize=(6.6, 4.0), dpi=200)
    ax.plot(n, n, color=COLORS_HEX[2], linewidth=2, label="O(n) — accès direct / voisinage borné")
    ax.plot(n, n ** 1.5, color=COLORS_HEX[3], linewidth=2, label="O(n^1.5) — recherche partielle")
    ax.plot(n, n ** 2, color=COLORS_HEX[0], linewidth=2, label="O(n²) — scan complet imbriqué")
    ax.set_xlabel("Taille relative des données (échelle arbitraire)")
    ax.set_ylabel("Coût relatif (échelle arbitraire)")
    ax.set_title("Rappel — pourquoi la complexité algorithmique domine\nà mesure que les données grandissent (schéma, non mesuré)", fontsize=10)
    ax.legend(fontsize=8, loc="upper left")
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.set_xticks([]); ax.set_yticks([])
    plt.tight_layout()
    plt.savefig(out_path)
    plt.close()


def make_hyperfine_chart(hyperfine, out_path):
    """Génère une vue des temps individuels et de la dispersion Hyperfine."""
    if not hyperfine or not hyperfine.get("results"):
        return False
    result = hyperfine["results"][0]
    times = result.get("times") or []
    if not times:
        return False
    fig, ax = plt.subplots(figsize=(6.6, 3.8), dpi=200)
    runs = list(range(1, len(times) + 1))
    ax.plot(runs, [value * 1000 for value in times], marker="o", linewidth=2,
            color=COLORS_HEX[1], label="Temps par run")
    ax.axhline(result.get("mean", 0) * 1000, color=COLORS_HEX[0], linestyle="--",
               linewidth=1.5, label=f"Moyenne ({result.get('mean', 0) * 1000:.2f} ms)")
    ax.set_xlabel("Execution Hyperfine")
    ax.set_ylabel("Temps (ms)")
    ax.set_title("Hyperfine — dispersion des temps de processus")
    ax.set_xticks(runs)
    ax.grid(axis="y", alpha=0.25)
    ax.legend(fontsize=8)
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    plt.tight_layout()
    plt.savefig(out_path)
    plt.close()
    return True


def make_go_samples_chart(results, out_path):
    """Compare les échantillons Go ns/op, B/op et allocations/op."""
    if not results:
        return False
    result = results[0]
    ns = result.get("nsSamples") or []
    bytes_per_op = result.get("bytesSamples") or []
    allocs = result.get("allocsSamples") or []
    if not ns:
        return False
    runs = list(range(1, len(ns) + 1))
    fig, axes = plt.subplots(3, 1, figsize=(6.8, 6.4), dpi=200, sharex=True)
    series = [
        (ns, "ns/op", "Temps Go", COLORS_HEX[0]),
        (bytes_per_op, "B/op", "Mémoire allouée", COLORS_HEX[1]),
        (allocs, "allocs/op", "Allocations", COLORS_HEX[2]),
    ]
    for axis, (values, unit, title, color) in zip(axes, series):
        if values:
            axis.plot(runs[:len(values)], values, marker="o", color=color, linewidth=1.8)
            axis.axhline(sum(values) / len(values), color="#777777", linestyle="--", linewidth=1)
        axis.set_ylabel(unit)
        axis.set_title(title, loc="left", fontsize=9)
        axis.grid(axis="y", alpha=0.2)
        axis.spines["top"].set_visible(False)
        axis.spines["right"].set_visible(False)
    axes[-1].set_xlabel("Execution Go")
    axes[-1].set_xticks(runs)
    fig.suptitle("Go benchmark — stabilité des mesures", fontsize=11)
    plt.tight_layout()
    plt.savefig(out_path)
    plt.close()
    return True


# ========================================================================
# 4. Construction du PDF
# ========================================================================

def fmt_pct(part, total):
    return f"{(part / total * 100):.1f}%" if total else "—"


def register_pdf_fonts():
    candidates = [
        (r"C:\Windows\Fonts\arial.ttf", r"C:\Windows\Fonts\arialbd.ttf", r"C:\Windows\Fonts\ariali.ttf"),
        (r"C:\Windows\Fonts\segoeui.ttf", r"C:\Windows\Fonts\segoeuib.ttf", r"C:\Windows\Fonts\segoeuii.ttf"),
    ]
    for regular, bold, italic in candidates:
        if all(os.path.isfile(path) for path in (regular, bold, italic)):
            pdfmetrics.registerFont(TTFont("BenchmarkRegular", regular))
            pdfmetrics.registerFont(TTFont("BenchmarkBold", bold))
            pdfmetrics.registerFont(TTFont("BenchmarkItalic", italic))
            return {"regular": "BenchmarkRegular", "bold": "BenchmarkBold", "italic": "BenchmarkItalic"}
    return {"regular": "Helvetica", "bold": "Helvetica-Bold", "italic": "Helvetica-Oblique"}


def build_pdf(out_pdf, ctx):
    fonts = register_pdf_fonts()
    styles = getSampleStyleSheet()
    DARK = colors.HexColor("#2B2B2B")
    GREY = colors.HexColor("#666666")
    RED = colors.HexColor("#D64545")
    LIGHTBG = colors.HexColor("#F4F4F4")

    def add_style(name, **kw):
        if name not in styles:
            styles.add(ParagraphStyle(name=name, **kw))

    add_style("TitleBig", fontSize=22, leading=26, textColor=DARK, spaceAfter=4, fontName=fonts["bold"])
    add_style("Subtitle", fontSize=11.5, leading=15, textColor=GREY, spaceAfter=14)
    add_style("H1", fontSize=15, leading=18, textColor=DARK, spaceBefore=16, spaceAfter=8, fontName=fonts["bold"])
    add_style("H2", fontSize=12, leading=15, textColor=RED, spaceBefore=10, spaceAfter=6, fontName=fonts["bold"])
    add_style("Body", fontSize=10, leading=14.5, textColor=DARK, spaceAfter=6, alignment=TA_LEFT)
    add_style("BodySmall", fontSize=8.7, leading=12, textColor=GREY, spaceAfter=4)
    add_style("Caption", fontSize=8.5, leading=11, textColor=GREY, alignment=TA_CENTER,
              spaceBefore=4, spaceAfter=10, fontName=fonts["italic"])
    add_style("MyBullet", fontSize=10, leading=14.5, textColor=DARK, leftIndent=12, spaceAfter=4)

    doc = SimpleDocTemplate(out_pdf, pagesize=A4,
                             leftMargin=18 * mm, rightMargin=18 * mm,
                             topMargin=16 * mm, bottomMargin=16 * mm,
                             title="Rapport de benchmark", author="generate_benchmark_report.py")
    story = []

    def hr(space_before=4, space_after=10):
        story.append(Spacer(1, space_before))
        story.append(HRFlowable(width="100%", thickness=0.7, color=colors.HexColor("#DDDDDD")))
        story.append(Spacer(1, space_after))

    def bullet(html):
        story.append(Paragraph(f"•&nbsp;&nbsp;{html}", styles["MyBullet"]))

    def styled_table(data, col_widths, header=True):
        t = Table(data, colWidths=col_widths)
        style = [
            ("FONTSIZE", (0, 0), (-1, -1), 9),
            ("GRID", (0, 0), (-1, -1), 0.4, colors.HexColor("#DDDDDD")),
            ("TOPPADDING", (0, 0), (-1, -1), 5),
            ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
            ("LEFTPADDING", (0, 0), (-1, -1), 8),
        ]
        if header:
            style += [
                ("FONTNAME", (0, 0), (-1, 0), fonts["bold"]),
                ("BACKGROUND", (0, 0), (-1, 0), DARK),
                ("TEXTCOLOR", (0, 0), (-1, 0), colors.white),
                ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, LIGHTBG]),
            ]
        else:
            style += [("ROWBACKGROUNDS", (0, 0), (-1, -1), [colors.white, LIGHTBG]),
                       ("FONTNAME", (0, 0), (0, -1), fonts["bold"])]
        t.setStyle(TableStyle(style))
        return t

    cfg = ctx["config"]
    gen_time = ctx["generated_at"]

    # ---------------- Cover ----------------
    story.append(Paragraph("Rapport de benchmark", styles["TitleBig"]))
    story.append(Paragraph(ctx.get("subtitle", "Rapport généré automatiquement"), styles["Subtitle"]))

    meta_rows = [["Généré le", gen_time]]
    if cfg.get("command"):
        meta_rows.append(["Commande", cfg["command"]])
    if cfg.get("goos") or cfg.get("goarch"):
        meta_rows.append(["Plateforme", f"{cfg.get('goos','?')}/{cfg.get('goarch','?')}"])
    if cfg.get("cpu"):
        meta_rows.append(["Processeur", cfg["cpu"].strip()])
    if cfg.get("benchmarkWidth") and cfg.get("benchmarkHeight"):
        density = cfg.get("benchmarkDensity", "n/a")
        meta_rows.append(["Carte benchmark", f"{cfg['benchmarkWidth']} × {cfg['benchmarkHeight']} (densité {density})"])
    if ctx.get("cpu_prof"):
        meta_rows.append(["Profil CPU", os.path.basename(ctx["cpu_prof"])])
    if ctx.get("mem_prof"):
        meta_rows.append(["Profil mémoire", os.path.basename(ctx["mem_prof"])])
    story.append(styled_table(meta_rows, [40 * mm, 122 * mm], header=False))
    hr(space_before=14)

    # ---------------- 1. Résultats bruts ----------------
    story.append(Paragraph("1. Résultats du benchmark", styles["H1"]))
    if ctx.get("bench_results"):
        rows = [["Benchmark", "Moyenne", "ns/op", "B/op", "allocs/op"]]
        for r in ctx["bench_results"]:
            rows.append([
                r.get("name", "—"),
                f"{r.get('meanSeconds', 0) * 1000:.2f} ms" if r.get("meanSeconds") is not None else "—",
                f"{r.get('nsPerOp', '—')}",
                f"{r.get('bytesPerOp', '—')}",
                f"{r.get('allocsPerOp', '—')}",
            ])
        story.append(styled_table(rows, [48 * mm, 30 * mm, 32 * mm, 26 * mm, 26 * mm]))
    else:
        story.append(Paragraph("Aucun résultat de benchmark structuré trouvé dans latest.json.", styles["Body"]))

    if ctx.get("hyperfine"):
        hres = ctx["hyperfine"]["results"][0] if ctx["hyperfine"].get("results") else {}
        story.append(Spacer(1, 8))
        story.append(Paragraph("Détail hyperfine", styles["H2"]))
        hf_rows = [["Métrique", "Valeur"]]
        for k, label in [("mean", "Moyenne"), ("stddev", "Écart-type"), ("median", "Médiane"),
                          ("user", "Temps utilisateur"), ("system", "Temps système"),
                          ("min", "Min"), ("max", "Max")]:
            v = hres.get(k)
            hf_rows.append([label, f"{v:.4f} s" if isinstance(v, (int, float)) else "n/a"])
        times = hres.get("times") or []
        memory_samples = hres.get("memory_usage_byte") or []
        if times:
            hf_rows.append(["Runs mesurés", str(len(times))])
            mean = hres.get("mean")
            stddev = hres.get("stddev")
            if isinstance(mean, (int, float)) and mean > 0 and isinstance(stddev, (int, float)):
                hf_rows.append(["Coefficient de variation", f"{(stddev / mean * 100):.2f}%"])
            else:
                hf_rows.append(["Coefficient de variation", "n/a"])
        if memory_samples and any(memory_samples):
            hf_rows.append(["Pic mémoire Hyperfine", f"{max(memory_samples) / 1024 / 1024:.2f} MiB"])
        elif cfg.get("processMemory", {}).get("peakWorkingSetBytes"):
            process_memory = cfg["processMemory"]
            hf_rows.append(["Pic WorkingSet commande Go", f"{process_memory['peakWorkingSetBytes'] / 1024 / 1024:.2f} MiB"])
            hf_rows.append(["Pic mémoire privée commande Go", f"{process_memory.get('peakPrivateBytes', 0) / 1024 / 1024:.2f} MiB"])
        else:
            hf_rows.append(["Mémoire Hyperfine", "Indisponible sur cette exécution"])
        story.append(styled_table(hf_rows, [60 * mm, 102 * mm]))
        if ctx.get("hyperfine_chart"):
            story.append(Spacer(1, 8))
            story.append(Image(ctx["hyperfine_chart"], width=145 * mm, height=84 * mm))
            story.append(Paragraph("Figure — Temps de chaque exécution Hyperfine et moyenne.", styles["Caption"]))

    if ctx.get("go_samples_chart"):
        story.append(Spacer(1, 8))
        story.append(Image(ctx["go_samples_chart"], width=145 * mm, height=137 * mm))
        story.append(Paragraph("Figure — Dispersion des mesures Go : temps, mémoire allouée et allocations.", styles["Caption"]))

    story.append(Spacer(1, 8))
    story.append(Paragraph("Résumé CPU et mémoire", styles["H2"]))
    resource_rows = [["Mesure", "Valeur"]]
    cpu_profile = ctx.get("cpu_parsed")
    memory_profile = ctx.get("mem_parsed")
    memory_inuse_profile = ctx.get("mem_inuse_parsed")
    if cpu_profile:
        resource_rows.append(["CPU échantillonné (pprof)", f"{cpu_profile['total_value']:.2f}{cpu_profile['total_unit']}"])
    if memory_profile:
        resource_rows.append(["Mémoire allouée (pprof)", f"{memory_profile['total_value']:.2f}{memory_profile['total_unit']}"])
    if memory_inuse_profile:
        resource_rows.append(["Heap vivant (pprof)", f"{memory_inuse_profile['total_value']:.2f}{memory_inuse_profile['total_unit']}"])
    system_memory = cfg.get("systemMemory") or {}
    if system_memory.get("totalBytes"):
        total_mib = system_memory["totalBytes"] / 1024 / 1024
        available_mib = system_memory.get("availableBytes", 0) / 1024 / 1024
        used_mib = system_memory.get("usedBytes", 0) / 1024 / 1024
        resource_rows.extend([
            ["RAM physique totale", f"{total_mib:.0f} MiB"],
            ["RAM physique disponible", f"{available_mib:.0f} MiB"],
            ["RAM physique utilisée", f"{used_mib:.0f} MiB"],
        ])
    for result in ctx.get("bench_results", []):
        resource_rows.append(["Mémoire par opération", f"{result.get('bytesPerOp', '—')} B/op"])
    if len(resource_rows) > 1:
        story.append(styled_table(resource_rows, [75 * mm, 87 * mm]))
    else:
        story.append(Paragraph("Aucune mesure CPU/mémoire disponible.", styles["Body"]))

    if ctx.get("gc_trace"):
        gc = ctx["gc_trace"]
        story.append(Spacer(1, 8))
        story.append(Paragraph("Garbage collector Go", styles["H2"]))
        gc_rows = [
            ["Cycles GC", str(gc["cycles"])],
            ["Temps GC cumulé", f"{gc['totalPauseMs']:.3f} ms"],
            ["Plus longue pause", f"{gc['maxPauseMs']:.3f} ms"],
            ["Temps CPU GC cumulé", f"{gc['totalCpuMs']:.3f} ms"],
            ["Objectif mémoire maximal", f"{gc['maxGoalMb']:.2f} MB"],
        ]
        story.append(styled_table(gc_rows, [75 * mm, 87 * mm], header=False))

    story.append(Spacer(1, 6))
    story.append(Paragraph(
        "La RAM physique est l'état du système au lancement du benchmark. La mémoire pprof "
        "mesure le heap Go échantillonné (alloué ou encore vivant), pas toute la mémoire RSS "
        "du processus. Hyperfine peut fournir le RSS quand le backend système le supporte ; "
        "une valeur nulle est donc signalée comme indisponible.",
        styles["BodySmall"]))

    # ---------------- 2. CPU ----------------
    if ctx.get("cpu_parsed") and ctx["cpu_parsed"]["rows"]:
        cp = ctx["cpu_parsed"]
        story.append(Paragraph("2. Profil CPU — où passe le temps ?", styles["H1"]))
        story.append(Paragraph(
            f"Type de profil : <b>{cp['type']}</b>. Total échantillonné : "
            f"<b>{cp['total_value']:.2f}{cp['total_unit']}</b>.", styles["Body"]))

        top_rows = sorted(cp["rows"], key=lambda r: r["flat_val"], reverse=True)[:8]
        rows = [["Fonction", "Flat", "Flat %", "Cumulé", "Cumulé %"]]
        for r in top_rows:
            rows.append([r["func"][:46], r["flat_str"], f"{r['flat_pct']:.2f}%",
                         r["cum_str"], f"{r['cum_pct']:.2f}%"])
        story.append(styled_table(rows, [70 * mm, 22 * mm, 20 * mm, 22 * mm, 22 * mm]))
        story.append(Spacer(1, 8))

        if ctx.get("cpu_chart"):
            story.append(Image(ctx["cpu_chart"], width=140 * mm, height=91.9 * mm))
            story.append(Paragraph("Figure — Répartition du temps CPU par fonction (temps flat, top fonctions).", styles["Caption"]))

        if ctx.get("cpu_graph_png"):
            story.append(Image(ctx["cpu_graph_png"], width=150 * mm, height=150 * mm * (ctx["cpu_graph_size"][1] / ctx["cpu_graph_size"][0])))
            story.append(Paragraph("Figure — Graphe d'appels CPU (go tool pprof -png).", styles["Caption"]))
        else:
            story.append(Paragraph(
                "(Graphe d'appel visuel non généré — Graphviz/'dot' introuvable dans le PATH. "
                "Installez Graphviz pour l'obtenir automatiquement au prochain run.)", styles["BodySmall"]))

        # Diagnostic automatique générique
        story.append(Paragraph("2.1 Diagnostic automatique", styles["H2"]))
        top1 = top_rows[0]
        story.append(Paragraph(
            f"La fonction dominante de ce run est <b>{top1['func']}</b>, responsable de "
            f"<b>{top1['flat_pct']:.1f}%</b> du temps CPU total (flat), pour "
            f"<b>{top1['cum_pct']:.1f}%</b> cumulé avec ses appels. C'est la cible prioritaire "
            f"d'optimisation pour ce run.", styles["Body"]))

        HOT_KEYWORDS = ["distance", "neighbor", "voisin", "infect", "search", "scan", "find"]
        if any(k in top1["func"].lower() for k in HOT_KEYWORDS):
            bullet("Le nom de cette fonction suggère une recherche/un parcours répété "
                   "(distance, voisinage, scan…) — vérifier si elle itère sur l'ensemble des "
                   "données à chaque appel plutôt que sur un sous-ensemble borné "
                   "(structure spatiale : grille, quadtree, index).")
        bullet("Comparer ce hotspot avec le run précédent : s'il a changé de fonction, "
               "l'optimisation précédente a fonctionné et un nouveau goulot est apparu ailleurs.")
        bullet("Vérifier s'il existe un algorithme en O(n) ou O(log n) équivalent pour cette "
               "opération avant d'optimiser au niveau micro (inlining, allocations).")

        if ctx.get("complexity_chart"):
            story.append(Image(ctx["complexity_chart"], width=135 * mm, height=81.8 * mm))
            story.append(Paragraph("Figure — Rappel pédagogique sur la complexité algorithmique (schéma générique).", styles["Caption"]))

        story.append(PageBreak())

    # ---------------- 3. Mémoire ----------------
    if ctx.get("mem_parsed") and ctx["mem_parsed"]["rows"]:
        mp = ctx["mem_parsed"]
        story.append(Paragraph("3. Profil mémoire — allocations", styles["H1"]))
        story.append(Paragraph(
            f"Type de profil : <b>{mp['type']}</b>. Total : "
            f"<b>{mp['total_value']:.2f}{mp['total_unit']}</b>.", styles["Body"]))

        top_rows = sorted(mp["rows"], key=lambda r: r["flat_val"], reverse=True)[:8]
        rows = [["Fonction", "Flat", "Flat %", "Cumulé", "Cumulé %"]]
        for r in top_rows:
            rows.append([r["func"][:46], r["flat_str"], f"{r['flat_pct']:.2f}%",
                         r["cum_str"], f"{r['cum_pct']:.2f}%"])
        story.append(styled_table(rows, [70 * mm, 22 * mm, 20 * mm, 22 * mm, 22 * mm]))
        story.append(Spacer(1, 8))

        if ctx.get("mem_chart"):
            story.append(Image(ctx["mem_chart"], width=140 * mm, height=91.9 * mm))
            story.append(Paragraph("Figure — Répartition des allocations mémoire par fonction.", styles["Caption"]))

        if ctx.get("mem_inuse_chart"):
            story.append(Image(ctx["mem_inuse_chart"], width=140 * mm, height=91.9 * mm))
            story.append(Paragraph("Figure — Heap Go encore vivant par fonction (pprof -inuse_space).", styles["Caption"]))

        if ctx.get("mem_graph_png"):
            story.append(Image(ctx["mem_graph_png"], width=150 * mm, height=150 * mm * (ctx["mem_graph_size"][1] / ctx["mem_graph_size"][0])))
            story.append(Paragraph("Figure — Graphe d'appels mémoire (go tool pprof -png).", styles["Caption"]))
        story.append(PageBreak())

    # ---------------- 4. Limites (générées dynamiquement selon config) ----------------
    story.append(Paragraph("4. Limites de ce benchmark & informations manquantes", styles["H1"]))
    story.append(Paragraph("Générées automatiquement à partir de la configuration détectée :", styles["Body"]))

    runs = cfg.get("runs")
    warmup = cfg.get("warmup")
    if runs is not None and runs < 5:
        bullet(f"<b>Peu de runs ({runs}).</b> Avec moins de 5 exécutions, aucune médiane/écart-type "
               "fiable n'est disponible. Recommandé : ≥ 10 runs (<code>-count=10</code> ou "
               "hyperfine <code>--min-runs 10</code>).")
    if warmup is not None and warmup == 0:
        bullet("<b>Aucun warmup.</b> La première exécution peut être biaisée (cache froid, "
               "JIT du GC non stabilisé). Ajouter 1-2 warmups avant la mesure.")
    bullet("<b>Pas de comparaison avant/après</b> intégrée à ce rapport — conservez les PDF "
           "successifs pour comparer les hotspots et le temps moyen dans le temps.")
    bullet("<b>Un seul point de taille de données.</b> Si applicable, mesurer plusieurs tailles "
           "permettrait de vérifier empiriquement la complexité algorithmique observée en section 2.")
    if not ctx.get("gc_trace") or ctx["gc_trace"]["cycles"] == 0:
        bullet("<b>Trace GC indisponible</b> (<code>GODEBUG=gctrace=1</code>) — relancer le "
               "benchmark avec la collecte GC activée.")

    if ctx.get("md_notes"):
        story.append(Paragraph("Notes de l'outil de benchmark", styles["H2"]))
        for n in ctx["md_notes"]:
            bullet(n)

    hr()
    story.append(Paragraph(
        f"Rapport généré automatiquement le {datetime.now().strftime('%Y-%m-%d %H:%M:%S')} par "
        f"generate_benchmark_report.py, à partir des fichiers les plus récents du dossier "
        f"de benchmarks. Analyse réalisée avec <font face='{fonts['regular']}'>go tool pprof</font>.",
        styles["BodySmall"]))

    doc.build(story)


# ========================================================================
# 5. Orchestration
# ========================================================================

def main():
    ap = argparse.ArgumentParser(description="Génère un rapport PDF complet à partir des derniers fichiers de benchmark.")
    ap.add_argument("--dir", default=".", help="Dossier contenant cpu-*.prof, memory-*.prof, hyperfine-*.json, latest.json, latest.md")
    ap.add_argument("--output", default=None, help="Chemin du PDF de sortie (par défaut : <dir>/pdf/benchmark-report-<horodatage>.pdf)")
    ap.add_argument("--keep-temp", action="store_true", help="Conserve les fichiers intermédiaires (graphiques PNG) pour debug")
    args = ap.parse_args()

    directory = os.path.abspath(args.dir)
    if not os.path.isdir(directory):
        sys.exit(f"[generate_benchmark_report] Dossier introuvable : {directory}")

    cpu_prof = find_latest(directory, "cpu-*.prof")
    mem_prof = find_latest(directory, "memory-*.prof")
    gc_log = find_latest(directory, "gc-*.log")
    hyperfine_path = find_latest(directory, "hyperfine-*.json")
    latest_json_path = os.path.join(directory, "latest.json")
    latest_md_path = os.path.join(directory, "latest.md")

    latest = load_json(latest_json_path) or {}
    hyperfine = load_json(hyperfine_path) if hyperfine_path else None
    md_notes = extract_notes_from_md(latest_md_path)

    config = latest.get("configuration", {}) or {}
    bench_results = latest.get("results", []) or []
    generated_at = latest.get("generatedAt", datetime.now(timezone.utc).isoformat())

    tmpdir = tempfile.mkdtemp(prefix="bench_report_")
    ctx = {
        "config": config,
        "generated_at": generated_at,
        "bench_results": bench_results,
        "hyperfine": hyperfine,
        "cpu_prof": cpu_prof,
        "mem_prof": mem_prof,
        "gc_trace": parse_gc_trace(gc_log),
        "md_notes": md_notes,
        "subtitle": f"{bench_results[0]['name']} — analyse CPU, mémoire et hyperfine" if bench_results else "Analyse CPU, mémoire et hyperfine",
    }

    # CPU
    if cpu_prof:
        cpu_text = run_pprof_top(cpu_prof)
        cpu_parsed = parse_pprof_top(cpu_text)
        ctx["cpu_parsed"] = cpu_parsed
        if cpu_parsed and cpu_parsed["rows"]:
            chart_path = os.path.join(tmpdir, "cpu_chart.png")
            if make_breakdown_chart(cpu_parsed, chart_path,
                                     "Répartition du temps CPU par fonction (top)",
                                     "Temps CPU cumulé"):
                ctx["cpu_chart"] = chart_path
        png_path = os.path.join(tmpdir, "cpu_graph.png")
        if run_pprof_png(cpu_prof, png_path):
            from PIL import Image as PILImage
            ctx["cpu_graph_png"] = png_path
            ctx["cpu_graph_size"] = PILImage.open(png_path).size

    # Memory
    if mem_prof:
        mem_text = run_pprof_top(mem_prof, extra_flag="-alloc_space")
        mem_parsed = parse_pprof_top(mem_text)
        ctx["mem_parsed"] = mem_parsed
        if mem_parsed and mem_parsed["rows"]:
            chart_path = os.path.join(tmpdir, "mem_chart.png")
            if make_breakdown_chart(mem_parsed, chart_path,
                                     "Répartition des allocations mémoire par fonction (top)",
                                     "Mémoire allouée cumulée"):
                ctx["mem_chart"] = chart_path
        inuse_text = run_pprof_top(mem_prof, extra_flag="-inuse_space")
        mem_inuse_parsed = parse_pprof_top(inuse_text)
        ctx["mem_inuse_parsed"] = mem_inuse_parsed
        if mem_inuse_parsed and mem_inuse_parsed["rows"]:
            chart_path = os.path.join(tmpdir, "mem_inuse_chart.png")
            if make_breakdown_chart(mem_inuse_parsed, chart_path,
                                     "Heap Go vivant par fonction (pprof)",
                                     "Heap vivant"):
                ctx["mem_inuse_chart"] = chart_path
        png_path = os.path.join(tmpdir, "mem_graph.png")
        if run_pprof_png(mem_prof, png_path, extra_flag="-alloc_space"):
            from PIL import Image as PILImage
            ctx["mem_graph_png"] = png_path
            ctx["mem_graph_size"] = PILImage.open(png_path).size

    complexity_path = os.path.join(tmpdir, "complexity.png")
    make_complexity_illustration(complexity_path)
    ctx["complexity_chart"] = complexity_path

    hyperfine_chart_path = os.path.join(tmpdir, "hyperfine_chart.png")
    if make_hyperfine_chart(hyperfine, hyperfine_chart_path):
        ctx["hyperfine_chart"] = hyperfine_chart_path
    go_samples_chart_path = os.path.join(tmpdir, "go_samples_chart.png")
    if make_go_samples_chart(bench_results, go_samples_chart_path):
        ctx["go_samples_chart"] = go_samples_chart_path

    if args.output:
        out_pdf = args.output
    else:
        ts = re.sub(r"[^0-9]", "", generated_at)[:17] or datetime.now().strftime("%Y%m%d%H%M%S")
        out_pdf = os.path.join(directory, "pdf", f"benchmark-report-{ts}.pdf")

    os.makedirs(os.path.dirname(os.path.abspath(out_pdf)), exist_ok=True)

    build_pdf(out_pdf, ctx)

    if not args.keep_temp:
        shutil.rmtree(tmpdir, ignore_errors=True)

    print(f"[generate_benchmark_report] Rapport généré : {out_pdf}")


if __name__ == "__main__":
    main()