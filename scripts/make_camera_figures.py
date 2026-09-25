#!/usr/bin/env python3
"""Build the camera-ready results figure (Fig. 2 of the paper) from data that is
committed in this artifact. Nothing is typed in by hand.

Inputs, under implementation/:
  experiments/outputs/processed/baseline-comparison-real-alibaba.csv
      unsafe-promotion rate per method on the real Alibaba topologies at six
      scales (from make baseline-compare)
  experiments/outputs/processed/staleness-sweep-results.csv
      the mass-staleness sweep on the 200-service mesh (from make staleness-sweep)
  experiments/fixtures/real-topologies/alibaba/alibaba_<N>.edges
      the real edge lists. The share of edges into the single top hub is computed
      from them here
  experiments/outputs/processed/pq-tls-handshake-results.csv
      the measured TLS 1.3 handshake overhead, 300 handshakes per group [M]
  experiments/outputs/processed/setup-cost-results.csv
      the modelled aggregate setup-cost range [P] (replayed by run-paper-results.py)

Panels:
  (a) RQ2. Unsafe-promotion rate against system size on the real topologies (one
      line per method), over the share of edges that end at the single
      highest-fan-in service (grey bars). Both are percentages of edges, which means they
      share one axis.
  (b) Mass staleness (Thm. 2). The unsafe-promotion rate of QTPO and of the
      provenance-blind ablation (lines), over QTPO's blocked-safe share (bars).
  (c) RQ3. The setup cost over classical TLS: the measured handshake term (solid)
      next to the modelled aggregate range (hatched), which means a modelled number is
      never drawn like a measured one.

Output: paper/figures/results-figure.pdf, which the camera-ready includes as
figures/results-figure.pdf.

Usage:
  python3 scripts/make_camera_figures.py [--out results-figure.pdf] [--no-tex]
By default the text is typeset with LaTeX (Computer Modern, to match LNCS). Pass
--no-tex on a machine without a TeX installation.
"""
import argparse
import csv
from collections import Counter
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.lines import Line2D  # noqa: E402
from matplotlib.patches import Patch  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
PROC = ROOT / "implementation/experiments/outputs/processed"
TOPO = ROOT / "implementation/experiments/fixtures/real-topologies/alibaba"

# One style per method, used identically in every panel. The colours come from
# the Okabe-Ito palette. Each is paired with its own marker and line style, which means
# the figure also reads in greyscale.
STYLE = {
    "QTKG":          dict(label="QTPO",   color="#0072B2", marker="o", ls="-",  lw=1.6, ms=3.6),
    "NoKG":          dict(label="NoKG",     color="#56B4E9", marker="s", ls="--", lw=1.1, ms=3.0),
    "OPP":           dict(label="OPP",           color="#CC79A7", marker="P", ls="--", lw=1.0, ms=3.4),
    "ConfigProfile": dict(label="ConfigProf.", color="#E69F00", marker="^", ls="-.", lw=1.1, ms=3.4),
    "Static":        dict(label="Static",        color="#555555", marker="x", ls=":",  lw=1.1, ms=3.4),
    "ELCA":          dict(label="ELCA",          color="#009E73", marker="D", ls=":",  lw=1.1, ms=2.8),
    "SL-CA":         dict(label="SL-CA",         color="#D55E00", marker="v", ls="-",  lw=1.1, ms=3.4),
}
ORDER = ["QTKG", "NoKG", "OPP", "ConfigProfile", "Static", "ELCA", "SL-CA"]  # Table 1 order
HUB_FILL = "#D9D9D9"
BLOCK_FILL = "#EAF2F9"
# Panel (c) reuses the colours of the paper's provenance tags: green for [M] measured
# (green!40!black) and orange for [P] modelled (orange!75!black). The modelled range
# keeps its hatch, which means the two still differ in greyscale print.
MEAS_EDGE, MEAS_FILL = "#006600", "#2E8B3F"
MODEL_EDGE, MODEL_FILL = "#BF6000", "#FCE6CC"


def read_csv(path):
    """Return the rows of a CSV file as dictionaries."""
    with open(path, newline="") as fh:
        return list(csv.DictReader(fh))


def hub_share(scale):
    """Percent of edges whose destination is the single highest-fan-in service."""
    dests = []
    with open(TOPO / f"alibaba_{scale}.edges") as fh:
        for line in fh:
            parts = line.split()
            if len(parts) >= 2 and not line.startswith("#"):
                dests.append(parts[1])
    top = Counter(dests).most_common(1)[0][1]
    return 100.0 * top / len(dests)


def style_line(key):
    """Return the matplotlib line arguments for a method. QTPO is drawn on top."""
    s = STYLE[key]
    return dict(color=s["color"], marker=s["marker"], ls=s["ls"], lw=s["lw"],
                ms=s["ms"], mfc=s["color"], mec=s["color"], mew=0.8,
                clip_on=False, zorder=4 if key == "QTKG" else 3)


def main():
    """Read the data, draw the three panels and the shared legends and save the PDF."""
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(ROOT / "paper/figures/results-figure.pdf"))
    ap.add_argument("--no-tex", action="store_true")
    args = ap.parse_args()

    plt.rcParams.update({
        "font.size": 7.5, "axes.labelsize": 7.5, "axes.titlesize": 7.5,
        "xtick.labelsize": 7, "ytick.labelsize": 7, "legend.fontsize": 7,
        "axes.linewidth": 0.6, "xtick.major.width": 0.6, "ytick.major.width": 0.6,
        "xtick.major.size": 2.5, "ytick.major.size": 2.5,
        "xtick.minor.size": 0, "hatch.linewidth": 0.5,
        "pdf.fonttype": 42, "savefig.pad_inches": 0.01,
    })
    if args.no_tex:
        plt.rcParams.update({"font.family": "serif", "mathtext.fontset": "cm"})
    else:
        plt.rcParams.update({"text.usetex": True, "font.family": "serif",
                             "text.latex.preamble": r"\usepackage[T1]{fontenc}"})

    # Data
    real = read_csv(PROC / "baseline-comparison-real-alibaba.csv")
    scales = sorted({int(r["scale"]) for r in real})
    unsafe = {m: [float(next(r["unsafe_promotion_pct"] for r in real
                             if r["method"] == m and int(r["scale"]) == s))
                  for s in scales] for m in ORDER}
    hubs = [hub_share(s) for s in scales]

    stale = read_csv(PROC / "staleness-sweep-results.csv")
    fracs = sorted({float(r["stale_fraction"]) for r in stale})
    pick = lambda m, f, col: float(next(r[col] for r in stale  # noqa: E731
                                        if r["method"] == m and float(r["stale_fraction"]) == f))
    xs = [100 * f for f in fracs]
    q_unsafe = [pick("QTKG", f, "unsafe_promotion_pct") for f in fracs]
    q_block = [pick("QTKG", f, "blocked_safe_pct_median") for f in fracs]
    n_unsafe = [pick("NoKG", f, "unsafe_promotion_pct") for f in fracs]

    # Panel (c) data: the measured handshake term and the modelled aggregate.
    hs = {r["mode"]: float(r["rel_overhead_pct"])
          for r in read_csv(PROC / "pq-tls-handshake-results.csv")}
    agg = {r["mode"]: (float(r["setup_cost_percent_low"]), float(r["setup_cost_percent_high"]))
           for r in read_csv(PROC / "setup-cost-results.csv")}
    cost_modes = [("hybrid", "hybrid_mlkem", "hybrid"), ("pure\nPQ", "pure_pq", "pure-pqc")]

    pct = r"\%" if not args.no_tex else "%"

    # Figure
    cm = 1 / 2.54
    fig, (ax1, ax2, ax3) = plt.subplots(
        1, 3, figsize=(12.2 * cm, 4.7 * cm),
        gridspec_kw=dict(width_ratios=[1.3, 1.0, 0.64], wspace=0.36))
    fig.subplots_adjust(left=0.078, right=0.995, bottom=0.185, top=0.735)

    # Panel (a): real topologies.
    for s, h in zip(scales, hubs):
        ax1.bar(s, h, width=s * 0.36, color=HUB_FILL, edgecolor="none", zorder=1)
    for m in ORDER:
        ax1.plot(scales, unsafe[m], **style_line(m))
    ax1.set_xscale("log")
    ax1.set_xticks(scales)
    ax1.set_xticklabels([str(s) for s in scales])
    ax1.minorticks_off()
    ax1.set_xlim(scales[0] / 1.35, scales[-1] * 1.35)
    ax1.set_xlabel("Services (log scale)")
    ax1.set_ylabel(f"{pct} of managed edges")
    ax1.set_title("(a) Real Alibaba graphs", loc="left", pad=3)

    # Panel (b): mass staleness.
    ax2.bar(xs, q_block, width=4.2, color=BLOCK_FILL, edgecolor=STYLE["QTKG"]["color"],
            linewidth=0.6, zorder=1)
    ax2.plot(xs, n_unsafe, **style_line("NoKG"))
    ax2.plot(xs, q_unsafe, **style_line("QTKG"))
    ax2.set_xticks([0, 25, 50, 75, 90])
    ax2.set_xlim(-4, 94)
    ax2.set_xlabel(f"Corrupted facts ({pct})")
    ax2.set_title("(b) Mass staleness", loc="left", pad=3)

    for ax in (ax1, ax2):
        ax.set_ylim(-4, 100)
        ax.set_yticks([0, 25, 50, 75, 100])

    # Panel (c): measured values are solid bars, and the modelled range is a
    # hatched floating bar.
    for i, (lab, hkey, akey) in enumerate(cost_modes):
        v = hs[hkey]
        lo, hi = agg[akey]
        ax3.bar(i - 0.19, v, width=0.34, color=MEAS_FILL, edgecolor=MEAS_EDGE, lw=0.5, zorder=2)
        ax3.text(i - 0.19, v + 0.4, f"{v:.1f}", ha="center", va="bottom", fontsize=7,
                 color=MEAS_EDGE)
        ax3.bar(i + 0.19, hi - lo, bottom=lo, width=0.34, color=MODEL_FILL,
                edgecolor=MODEL_EDGE, lw=0.6, hatch="//////", zorder=2)
        ax3.text(i + 0.19, hi + 0.4, f"{lo:g}--{hi:g}" if not args.no_tex else f"{lo:g}-{hi:g}",
                 ha="center", va="bottom", fontsize=7, color=MODEL_EDGE)
    ax3.set_xticks([0, 1])
    ax3.set_xticklabels([m[0] for m in cost_modes])
    ax3.set_xlim(-0.55, 1.55)
    ax3.set_ylim(0, 22)
    ax3.set_yticks([0, 5, 10, 15, 20])
    ax3.set_ylabel(f"{pct} over classical")
    ax3.set_title("(c) Setup cost", loc="left", pad=3)
    ax3.legend(handles=[Patch(facecolor=MEAS_FILL, edgecolor=MEAS_EDGE, label="measured"),
                        Patch(facecolor=MODEL_FILL, edgecolor=MODEL_EDGE, lw=0.6, hatch="//////",
                              label="modelled")],
               loc="upper left", frameon=False, handlelength=1.3, handletextpad=0.4,
               borderaxespad=0.1, labelspacing=0.2, bbox_to_anchor=(0.0, 1.02))

    for ax in (ax1, ax2, ax3):
        ax.grid(axis="y", color="#E5E5E5", lw=0.5, zorder=0)
        ax.set_axisbelow(True)
        for side in ("top", "right"):
            ax.spines[side].set_visible(False)
        ax.tick_params(pad=1.5)

    # Shared legend in the method order of Table 1. Lines always show the
    # unsafe-promotion rate. The bars in (a) and (b) show a context quantity that
    # differs per panel.
    handles = [Line2D([], [], **{k: v for k, v in style_line(m).items()
                                 if k not in ("clip_on", "zorder")},
                      label=STYLE[m]["label"]) for m in ORDER]
    leg1 = fig.legend(handles=handles, loc="upper center", ncol=7, frameon=False,
                      bbox_to_anchor=(0.5, 1.0), columnspacing=0.9, handlelength=2.0,
                      handletextpad=0.35, borderaxespad=0.1)
    bars = [Patch(facecolor=HUB_FILL, edgecolor="none",
                  label="(a) share of edges into the single top hub"),
            Patch(facecolor=BLOCK_FILL, edgecolor=STYLE["QTKG"]["color"], lw=0.6,
                  label="(b) QTPO blocked-safe share")]
    fig.legend(handles=bars, loc="upper center", ncol=2, frameon=False,
               bbox_to_anchor=(0.5, 0.9), columnspacing=1.6, handlelength=1.6,
               handletextpad=0.4, borderaxespad=0.1)
    fig.add_artist(leg1)

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(out)
    print(f"wrote {out}")
    print("hub share (%):", ", ".join(f"{s}:{h:.1f}" for s, h in zip(scales, hubs)))


if __name__ == "__main__":
    main()
