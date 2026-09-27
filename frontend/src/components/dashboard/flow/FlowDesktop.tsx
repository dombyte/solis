import React from 'react';
import type { FlowViewModel } from './model';
import { FlowEdge } from './FlowEdge';
import { CircleNode, CircleBatteryNode } from './CircleNode';
import { BatteryGauge } from './BatteryGauge';
import { InverterStatus } from './InverterStatus';

/**
 * Desktop flow diagram with circles + inverter box
 */
interface FlowDesktopProps {
  viewModel: FlowViewModel;
}

// Geometry: SVG 1000x850 (wider and taller for better visibility)
const SVG_WIDTH = 1000;
const SVG_HEIGHT = 850;  // Increased for more backup space

// Inverter box (center)
const C = { x: 500, y: 340 };
const INV = { 
  x: C.x - 115, 
  y: C.y - 85, 
  w: 230, 
  h: 170 
};
const INV_RX = 20;

// Node positions - scaled for larger SVG
const PV = { x: 150, y: 90 };
const GRID = { x: 850, y: 90 };
const BAT = { x: 150, y: 500 };
const HH = { x: 850, y: 480 };
const BK = { x: 500, y: 750 };  // Positioned below inverter

// Node radius - increased for better visibility
const R = 58;

// Mid Y for edge calculations
const MID_Y = C.y;

/**
 * Build edge path for vertical edges
 */
function buildVEdge(nx: number, ny: number, side: number): string {
  const ex = side < 0 ? INV.x : INV.x + INV.w;
  const level = ny < MID_Y ? INV.y + 44 : INV.y + INV.h - 44;
  const dir = ny < level ? 1 : -1;
  const cr = 20;
  const cx2 = nx + (side < 0 ? cr : -cr);
  
  return `M ${nx} ${ny} L ${nx} ${level - dir * cr} Q ${nx} ${level} ${cx2} ${level} L ${ex} ${level}`;
}

/**
 * Build edge path for backup (from inverter bottom to backup)
 */
function buildBkEdge(bx: number, by: number): string {
  // BK is centered below inverter, so path is straight down from inverter bottom to BK
  return `M ${C.x} ${INV.y + INV.h} L ${bx} ${INV.y + INV.h} L ${bx} ${by - R}`;
}

export function FlowDesktop({ viewModel }: FlowDesktopProps): React.ReactElement {
  const { nodes, edges, inverter, stale } = viewModel;

  // Edge paths
  const edgePaths = {
    pv_to_inverter: buildVEdge(PV.x, PV.y + R, -1),
    grid_to_inverter: buildVEdge(GRID.x, GRID.y + R, 1),
    battery_to_inverter: buildVEdge(BAT.x, BAT.y - R, -1),
    inverter_to_household: buildVEdge(HH.x, HH.y - R, 1),
    inverter_to_backup: buildBkEdge(BK.x, BK.y),
  };

  // Battery gauge element
  const batteryGauge = <BatteryGauge soc={nodes.battery.soc ?? 0} />;

  return (
    <svg 
      viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`} 
      className="w-full h-auto max-w-4xl mx-auto" 
      style={{
        opacity: stale ? 0.35 : 1,
        transition: 'opacity 0.5s ease'
      }}
    >
      {/* Edges */}
      <FlowEdge 
        d={edgePaths.pv_to_inverter}
        active={edges.pv_to_inverter.active}
        reverse={edges.pv_to_inverter.reverse}
        color="var(--color-flow-pv)"
      />
      <FlowEdge 
        d={edgePaths.grid_to_inverter}
        active={edges.grid_to_inverter.active}
        reverse={edges.grid_to_inverter.reverse}
        color="var(--color-flow-grid)"
      />
      <FlowEdge 
        d={edgePaths.battery_to_inverter}
        active={edges.battery_to_inverter.active}
        reverse={edges.battery_to_inverter.reverse}
        color="var(--color-flow-batt)"
      />
      <FlowEdge 
        d={edgePaths.inverter_to_household}
        active={edges.inverter_to_household.active}
        reverse={edges.inverter_to_household.reverse}
        color="var(--color-flow-hh)"
      />
      <FlowEdge 
        d={edgePaths.inverter_to_backup}
        active={edges.inverter_to_backup.active}
        reverse={false} // Always flows FROM inverter to backup
        color="var(--color-flow-bk)"
      />

      {/* Nodes */}
      <CircleNode 
        node={nodes.pv}
        cx={PV.x}
        cy={PV.y}
        r={R}
        icon="sun"
        labelPos="above"
      />
      <CircleNode 
        node={nodes.grid}
        cx={GRID.x}
        cy={GRID.y}
        r={R}
        icon="factory"
        labelPos="above"
      />
      <CircleNode 
        node={nodes.household}
        cx={HH.x}
        cy={HH.y}
        r={R}
        icon="house"
      />
      <CircleNode 
        node={nodes.backup}
        cx={BK.x}
        cy={BK.y}
        r={R}
        icon="shield"
      />
      <CircleBatteryNode 
        node={nodes.battery}
        cx={BAT.x}
        cy={BAT.y}
        r={R}
        batteryGauge={batteryGauge}
      />

      {/* Inverter box at center - use flow-inv-bg for distinct appearance */}
      <g 
        transform={`translate(${C.x}, ${C.y})`}
        style={{
          opacity: stale ? 0.35 : 1,
          transition: 'opacity 0.5s ease'
        }}
      >
        <rect 
          x={INV.x - C.x} 
          y={INV.y - C.y} 
          width={INV.w} 
          height={INV.h} 
          rx={INV_RX} 
          fill="var(--color-flow-inv-bg)"  // Use distinct inverter background
          stroke="var(--color-border)"  // Add border for definition
          strokeWidth={1.5}
        />
        <InverterStatus 
          status={inverter.status}
          operatingStatus={inverter.operatingStatus}
          alert={inverter.alert}
          stale={stale}
          showIcon={true}
        />
      </g>
    </svg>
  );
}
