import React from 'react';
import { Sun, Factory, Home, ShieldCheck } from 'lucide-react';
import type { NodeViewModel } from './model';

const ICON_SIZE = 24;

const NODE_ICONS = {
  sun: Sun,
  factory: Factory,
  house: Home,
  shield: ShieldCheck,
} as const;

/**
 * Renders a lucide-react node icon centered on (0, 0) within an SVG context.
 * `color` is passed straight through as lucide's `color` prop, which the icon uses
 * for its `stroke` (these are outline-only icons), so it must already resolve to a
 * concrete color (a `var(--color-*)` token, not a class), matching how the circle
 * border/text around it are colored.
 */
function NodeIcon({ name, color }: { name: keyof typeof NODE_ICONS; color: string }): React.ReactElement {
  const Icon = NODE_ICONS[name];
  return (
    <g transform={`translate(${-ICON_SIZE / 2}, ${-ICON_SIZE / 2})`}>
      <Icon size={ICON_SIZE} color={color} strokeWidth={2.5} />
    </g>
  );
}

/**
 * Circle node for desktop flow diagram
 */
interface CircleNodeProps {
  node: NodeViewModel;
  cx: number;
  cy: number;
  r: number;
  icon: keyof typeof NODE_ICONS;
  labelPos?: 'above' | 'below';
}

export function CircleNode({ 
  node, 
  cx, 
  cy, 
  r, 
  icon,
  labelPos = 'below' 
}: CircleNodeProps): React.ReactElement {
  const opacity = node.stale ? 0.35 : 1;
  const iconColor = node.stale ? 'var(--color-muted-foreground)' : `var(--color-${node.color.substring(2)})`;
  const ariaLabel = `${node.label}: ${node.stale ? 'no data' : node.displayValue}`;

  return (
    <g
      style={{
        opacity,
        transition: 'opacity 0.5s ease'
      }}
      role="img"
      aria-label={ariaLabel}
    >
      {/* Node circle with colored border */}
      <circle 
        cx={cx} 
        cy={cy} 
        r={r} 
        fill="var(--color-card)" 
        stroke={node.stale ? 'var(--color-border)' : `var(--color-${node.color.substring(2)})`}
        strokeWidth={2.5} 
      />
      
      {/* Title above the icon */}
      <text 
        x={cx} 
        y={cy - 26} 
        textAnchor="middle" 
        fill="var(--color-muted-foreground)" 
        style={{ 
          fontSize: 18, 
          fontWeight: 600,
          transition: 'opacity 0.5s ease'
        }}
      >
        {node.label}
      </text>
      
      {/* Icon in the center */}
      <g transform={`translate(${cx}, ${cy})`}>
        <NodeIcon name={icon} color={iconColor} />
      </g>
      
      {/* Value below (or above) the circle */}
      <text 
        x={cx} 
        y={labelPos === 'above' ? cy - r - 16 : cy + r + 26}
        textAnchor="middle" 
        fill="var(--color-foreground)" 
        style={{
          fontSize: 19, 
          fontWeight: 700,
          transition: 'opacity 0.5s ease'
        }}
      >
        {node.displayValue}
      </text>
    </g>
  );
}

/**
 * Special circle node for battery with SOC gauge
 */
interface CircleBatteryNodeProps {
  node: NodeViewModel;
  cx: number;
  cy: number;
  r: number;
  batteryGauge: React.ReactNode;
}

export function CircleBatteryNode({ 
  node, 
  cx, 
  cy, 
  r,
  batteryGauge 
}: CircleBatteryNodeProps): React.ReactElement {
  const opacity = node.stale ? 0.35 : 1;
  const soc = node.soc ?? 0;
  const ariaLabel = node.stale
    ? 'Battery: no data'
    : `Battery: ${soc.toFixed(0)}% charge, ${node.displayValue}`;

  return (
    <g
      style={{
        opacity,
        transition: 'opacity 0.5s ease'
      }}
      role="img"
      aria-label={ariaLabel}
    >
      {/* Node circle with battery color border */}
      <circle 
        cx={cx} 
        cy={cy} 
        r={r} 
        fill="var(--color-card)" 
        stroke="var(--color-flow-batt)" 
        strokeWidth={2.5} 
      />
      
      {/* Battery gauge (imported component) */}
      <g transform={`translate(${cx - 25}, ${cy - 18})`}>
        {batteryGauge}
      </g>
      
      {/* Battery terminal indicator */}
      <rect 
        x={cx + 21} 
        y={cy - 12} 
        width={4} 
        height={12} 
        rx={1.2} 
        fill="var(--color-flow-batt-icon-stroke)"
      />
      
      {/* Label */}
      <text 
        x={cx - 4} 
        y={cy - 32} 
        textAnchor="middle" 
        fill="var(--color-muted-foreground)" 
        style={{ 
          fontSize: 18, 
          fontWeight: 600,
          transition: 'opacity 0.5s ease'
        }}
      >
        Batt
      </text>
      
      {/* SOC percentage */}
      <text 
        x={cx} 
        y={cy + 30} 
        textAnchor="middle" 
        fill="var(--color-foreground)" 
        style={{
          fontSize: 18, 
          fontWeight: 700,
          transition: 'opacity 0.5s ease'
        }}
      >
        {soc.toFixed(0)}%
      </text>
      
      {/* Power value */}
      <text 
        x={cx} 
        y={cy + r + 24} 
        textAnchor="middle" 
        fill="var(--color-foreground)" 
        style={{
          fontSize: 19, 
          fontWeight: 700,
          transition: 'opacity 0.5s ease'
        }}
      >
        {node.displayValue}
      </text>
    </g>
  );
}
