import React from 'react';
import type { NodeViewModel } from './model';

/**
 * SVG Icon component that renders an icon at (0,0)
 * All icons are outline-only for consistent appearance in both light and dark modes
 * For use within SVG contexts only
 */
export function SvgIcon({ name, color }: { name: 'sun' | 'factory' | 'house' | 'shield' | 'battery' | 'inverter'; color: string }): React.ReactElement {
  switch (name) {
    case 'sun':
      // Sun icon - outline only, no fill
      return (
        <g transform="translate(-15, -15)">
          <circle cx={12} cy={12} r={5} fill="none" stroke={color} strokeWidth={2.5} />
          <path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41M6.34 17.66L7.75 16.25M16.25 7.75l-1.41 1.41" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    case 'factory':
      // Factory/grid icon - outline only
      return (
        <g transform="translate(-15, -15)">
          <rect x={8} y={4} width={8} height={16} rx={1} fill="none" stroke={color} strokeWidth={2.5} />
          <path d="M6 12h2M16 12h2M6 8h2M16 8h2M6 16h2M16 16h2" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    case 'house':
      // House icon - outline only, NO fill
      return (
        <g transform="translate(-15, -15)">
          <path d="M12 2L2 12h5v8h10v-8h5L12 2z" stroke={color} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" fill="none" />
          <path d="M8 12v8M12 12v8M16 12v8" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    case 'shield':
      // Shield icon for backup - outline only, NO fill
      return (
        <g transform="translate(-15, -15)">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" stroke={color} strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" fill="none" />
          <path d="M12 16v-4" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    case 'battery':
      // Battery icon
      return (
        <g transform="translate(-15, -15)">
          <rect x={6} y={8} width={12} height={8} rx={1} fill="none" stroke={color} strokeWidth={2.5} />
          <path d="M12 6v-2" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    case 'inverter':
      // Inverter icon - a box with AC/DC symbols
      return (
        <g transform="translate(-15, -15)">
          <rect x={8} y={6} width={8} height={12} rx={1} fill="none" stroke={color} strokeWidth={2.5} />
          <path d="M12 4v4M12 16v4" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
          <path d="M8 10h2M14 10h2" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
        </g>
      );
    default:
      return <></>;
  }
}

/**
 * Circle node for desktop flow diagram
 */
interface CircleNodeProps {
  node: NodeViewModel;
  cx: number;
  cy: number;
  r: number;
  icon: 'sun' | 'factory' | 'house' | 'shield' | 'battery';
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
  
  return (
    <g 
      style={{
        opacity,
        transition: 'opacity 0.5s ease'
      }}
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
        <SvgIcon name={icon} color={iconColor} />
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
  
  return (
    <g 
      style={{
        opacity,
        transition: 'opacity 0.5s ease'
      }}
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
