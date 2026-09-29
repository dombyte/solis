import React from 'react';
import { Zap } from 'lucide-react';

/**
 * Inverter hub of the desktop flow diagram: box + icon badge, title, a status pill for
 * solis_status and the operating status below. The content is HTML in a foreignObject so
 * long status strings wrap inside the box instead of overflowing it.
 */
interface InverterStatusProps {
  x: number;
  y: number;
  width: number;
  height: number;
  status: string;
  operatingStatus: string;
  alert: boolean;
}

const BOX_RX = 20;
const BADGE = 52;
// lucide has no inverter glyph; Zap reads as power conversion
const ICON_SIZE = 30;
const DOT = 8;

/**
 * Tint a color token for badge/pill backgrounds
 */
function tint(color: string, percent: number): string {
  return `color-mix(in oklab, ${color} ${percent}%, transparent)`;
}

export function InverterStatus({
  x,
  y,
  width,
  height,
  status,
  operatingStatus,
  alert,
}: InverterStatusProps): React.ReactElement {
  const statusColor = alert ? 'var(--color-flow-status-error)' : 'var(--color-flow-status-ok)';
  const iconColor = alert ? statusColor : 'var(--color-primary)';

  return (
    <g role="img" aria-label={`Inverter: ${status}, ${operatingStatus}`}>
      <rect
        x={x}
        y={y}
        width={width}
        height={height}
        rx={BOX_RX}
        fill="var(--color-flow-inv-bg)"
        stroke={alert ? statusColor : 'var(--color-border)'}
        strokeWidth={alert ? 2.5 : 2}
      />
      <foreignObject x={x} y={y} width={width} height={height}>
        <div className="flex h-full flex-col items-center justify-center gap-1.5 px-3 text-center">
          <div
            className="flex items-center justify-center rounded-xl"
            style={{ width: BADGE, height: BADGE, background: tint(iconColor, 16) }}
          >
            <Zap size={ICON_SIZE} color={iconColor} strokeWidth={2.25} aria-hidden="true" />
          </div>
          <div style={{ color: 'var(--color-flow-inv-title)', fontSize: 20, fontWeight: 700 }}>
            Inverter
          </div>
          <div
            className="flex max-w-full items-center gap-1.5 rounded-full px-2.5 py-0.5"
            style={{ background: tint(statusColor, 16), color: statusColor, fontSize: 14, fontWeight: 600 }}
          >
            <span
              className="shrink-0 rounded-full"
              style={{ width: DOT, height: DOT, background: statusColor }}
            />
            <span className="truncate">{status}</span>
          </div>
          <div className="leading-tight" style={{ color: 'var(--color-flow-inv-sub)', fontSize: 14 }}>
            {operatingStatus}
          </div>
        </div>
      </foreignObject>
    </g>
  );
}
