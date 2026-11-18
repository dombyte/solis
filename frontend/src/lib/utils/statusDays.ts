import { addDays, startOfDay } from 'date-fns';
import type { StatusLevel } from './status';

/** A stored (or live) status change: the state entered at `at`. */
export interface StatusChangeInput {
  at: Date;
  text: string;
  level: StatusLevel;
}

/** One state the register entered, until the next change (null: still active). */
export interface StatusChange extends StatusChangeInput {
  until: Date | null;
}

/** A stretch of one day spent in one state. */
export interface StatusSegment {
  from: Date;
  to: Date;
  text: string;
  level: StatusLevel;
}

/** Changes oldest first, consecutive repeats of a state merged (a reconnect can repeat one). */
export function toChanges(entries: StatusChangeInput[]): StatusChange[] {
  const merged = entries.filter((e, i) => i === 0 || e.text !== entries[i - 1].text);
  return merged.map((e, i) => ({ ...e, until: merged[i + 1]?.at ?? null }));
}

/**
 * The states of one local day up to now. Before the oldest known change the state is
 * unknown (gray), never guessed. Empty for a day that has not started yet.
 */
export function daySegments(changes: StatusChange[], day: Date, now: Date): StatusSegment[] {
  const dayStart = startOfDay(day);
  const dayEnd = new Date(Math.min(addDays(dayStart, 1).getTime(), now.getTime()));
  if (dayEnd <= dayStart) return [];
  const segments: StatusSegment[] = [];
  let cursor = dayStart;
  let current: Pick<StatusSegment, 'text' | 'level'> = { text: 'No data', level: 'unknown' };
  for (const c of changes) {
    if (c.at >= dayEnd) break;
    if (c.at > cursor) {
      segments.push({ from: cursor, to: c.at, ...current });
      cursor = c.at;
    }
    current = { text: c.text, level: c.level };
  }
  segments.push({ from: cursor, to: dayEnd, ...current });
  return segments;
}

const LEVEL_RANK: Record<StatusLevel, number> = { unknown: 0, ok: 1, notice: 2, fault: 3 };

/** One day of the overview bars. */
export interface StatusDay {
  day: Date;
  /** The worst level of the day (unknown only when nothing about it is known). */
  level: StatusLevel;
  /** Milliseconds spent in each level. */
  time: Record<StatusLevel, number>;
  /** Distinct fault/notice states of the day, for the tooltip. */
  issues: string[];
}

/** The worst level and the time per level of each day (oldest first). */
export function summarizeDays(changes: StatusChange[], days: Date[], now: Date): StatusDay[] {
  return days.map(day => {
    const time: Record<StatusLevel, number> = { ok: 0, notice: 0, fault: 0, unknown: 0 };
    const issues = new Set<string>();
    let level: StatusLevel = 'unknown';
    for (const s of daySegments(changes, day, now)) {
      time[s.level] += s.to.getTime() - s.from.getTime();
      if (s.level === 'fault' || s.level === 'notice') issues.add(s.text);
      if (s.level !== 'unknown' && LEVEL_RANK[s.level] > LEVEL_RANK[level]) level = s.level;
    }
    return { day, level, time, issues: [...issues] };
  });
}

/** Share of the known time without a fault (null when nothing is known). */
export function faultFreeShare(days: StatusDay[]): number | null {
  const known = days.reduce((n, d) => n + d.time.ok + d.time.notice + d.time.fault, 0);
  if (known === 0) return null;
  return 1 - days.reduce((n, d) => n + d.time.fault, 0) / known;
}
