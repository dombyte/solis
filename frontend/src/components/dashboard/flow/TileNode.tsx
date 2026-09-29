import React from 'react';
import type { LucideIcon } from 'lucide-react';
import type { NodeViewModel } from './model';

// Tile geometry (SVG units); TILE_H is shared with FlowDesktop for edge endpoints
const TILE_W = 260;
export const TILE_H = 110;
const TILE_RX = 16;
const TILE_PAD = 16;

// Icon badge: a tinted rounded square on the left, vertically centered
const BADGE = 60;
const BADGE_RX = 12;
const ICON_SIZE = 32;
const TEXT_X = TILE_PAD + BADGE + 14;

interface TileNodeProps {
  node: NodeViewModel;
  cx: number;
  cy: number;
  /** lucide-react icon drawn in the badge */
  icon: LucideIcon;
  /** Tile heading; defaults to the node label */
  title?: string;
  /** Optional second line under the value (e.g. "exporting") */
  sub?: string;
}

/**
 * Renders a lucide-react icon centered on (0, 0). `color` is passed straight through as
 * lucide's stroke color, so it must resolve to a concrete color (a `var(--color-*)` token).
 */
function NodeIcon({ icon: Icon, color }: { icon: LucideIcon; color: string }): React.ReactElement {
  return (
    <g transform={`translate(${-ICON_SIZE / 2}, ${-ICON_SIZE / 2})`}>
      <Icon size={ICON_SIZE} color={color} strokeWidth={2.25} />
    </g>
  );
}

/**
 * Rounded tile node for the desktop flow diagram, centered on (cx, cy):
 * icon badge on the left, title / value / optional sub-status on the right.
 */
export function TileNode({ node, cx, cy, icon, title, sub }: TileNodeProps): React.ReactElement {
  const color = node.stale ? 'var(--color-muted-foreground)' : `var(--color-${node.color.substring(2)})`;
  const heading = title ?? node.label;
  const ariaLabel = node.stale
    ? `${heading}: no data`
    : `${heading}: ${node.displayValue}${sub ? ` (${sub})` : ''}`;
  // Without a sub line the two remaining lines are re-centered vertically
  const shift = sub ? 0 : 10;

  return (
    <g
      transform={`translate(${cx - TILE_W / 2}, ${cy - TILE_H / 2})`}
      style={{ opacity: node.stale ? 0.35 : 1, transition: 'opacity 0.5s ease' }}
      role="img"
      aria-label={ariaLabel}
    >
      <rect
        width={TILE_W}
        height={TILE_H}
        rx={TILE_RX}
        fill="var(--color-card)"
        stroke={node.stale ? 'var(--color-border)' : color}
        strokeWidth={2.5}
      />

      <rect
        x={TILE_PAD}
        y={(TILE_H - BADGE) / 2}
        width={BADGE}
        height={BADGE}
        rx={BADGE_RX}
        fill={color}
        fillOpacity={0.14}
      />
      <g transform={`translate(${TILE_PAD + BADGE / 2}, ${TILE_H / 2})`}>
        <NodeIcon icon={icon} color={color} />
      </g>

      <text
        x={TEXT_X}
        y={34 + shift}
        fill="var(--color-muted-foreground)"
        style={{ fontSize: 17, fontWeight: 600 }}
      >
        {heading}
      </text>
      <text
        x={TEXT_X}
        y={64 + shift}
        fill="var(--color-foreground)"
        style={{ fontSize: 25, fontWeight: 700 }}
      >
        {node.stale ? '–' : node.displayValue}
      </text>
      {sub && (
        <text x={TEXT_X} y={88} fill="var(--color-muted-foreground)" style={{ fontSize: 15 }}>
          {sub}
        </text>
      )}
    </g>
  );
}
