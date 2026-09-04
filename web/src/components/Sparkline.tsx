import { useMemo } from 'react';

// SparklinePoint is a single (day, value) sample. `day` is a
// human-visible label that renders under a subset of X-axis ticks.
export type SparklinePoint = {
  day: string;
  value: number;
};

// Sparkline renders a dependency-free SVG polyline. The Y axis is
// auto-scaled to the max value with ~15% headroom so the line does not
// hug the top edge. Extracted from analytics.tsx so the Nudgeway and
// Meta tabs share the same visual language.
export function Sparkline({
  points,
  color,
  label,
  ariaLabel,
}: {
  points: SparklinePoint[];
  color: string;
  label: string;
  ariaLabel: string;
}) {
  const width = 640;
  const height = 140;
  const pad = 8;

  const geometry = useMemo(() => {
    if (points.length === 0) {
      return {
        path: '',
        area: '',
        maxY: 0,
        ticks: [] as Array<{ x: number; day: string }>,
        dots: [] as Array<{ x: number; y: number; day: string }>,
      };
    }
    const values = points.map((p) => p.value);
    const maxRaw = Math.max(...values, 1);
    const maxY = Math.max(1, Math.ceil(maxRaw * 1.15));
    const innerW = width - pad * 2;
    const stepX = points.length > 1 ? innerW / (points.length - 1) : 0;
    const innerH = height - pad * 2;
    const coords = points.map((p, i) => {
      const x = points.length > 1 ? pad + stepX * i : pad + innerW / 2;
      const y = pad + innerH - (p.value / maxY) * innerH;
      return { x, y, day: p.day };
    });
    const path = coords
      .map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x.toFixed(1)} ${c.y.toFixed(1)}`)
      .join(' ');
    const area =
      coords.length > 1
        ? `${path} L ${coords[coords.length - 1]!.x.toFixed(1)} ${(height - pad).toFixed(1)} L ${coords[0]!.x.toFixed(1)} ${(height - pad).toFixed(1)} Z`
        : '';
    const stride = Math.max(1, Math.ceil(coords.length / 6));
    const ticks = coords.filter((_, i) => i % stride === 0).map((c) => ({ x: c.x, day: c.day }));
    const dots = coords.map((c) => ({ x: c.x, y: c.y, day: c.day }));
    return { path, area, maxY, ticks, dots };
  }, [points]);

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-2 flex items-baseline justify-between">
        <div className="text-sm font-medium text-slate-700">{label}</div>
        <div className="text-xs text-slate-500">max {geometry.maxY.toLocaleString()}</div>
      </div>
      {points.length === 0 ? (
        <div className="flex h-[140px] items-center justify-center text-sm text-slate-400">
          No data in the selected range.
        </div>
      ) : (
        <svg
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={ariaLabel}
          className="w-full"
        >
          <path d={geometry.area} fill={color} opacity={0.15} />
          <path d={geometry.path} fill="none" stroke={color} strokeWidth={2} />
          {geometry.dots.map((d) => (
            <circle key={`dot-${d.day}`} cx={d.x} cy={d.y} r={2.5} fill={color} />
          ))}
          {geometry.ticks.map((t) => (
            <text
              key={t.day}
              x={t.x}
              y={height - 2}
              textAnchor="middle"
              className="fill-slate-400 text-[9px]"
            >
              {t.day.slice(5)}
            </text>
          ))}
        </svg>
      )}
    </div>
  );
}
