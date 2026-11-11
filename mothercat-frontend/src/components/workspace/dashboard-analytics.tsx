"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type { MouseEvent, ReactNode } from "react";

import { GlassModal } from "@/components/workspace/glass-modal";
import { useCompanyActivity, useMyActivity, usePending } from "@/lib/analytics/hooks";
import type {
  AnalyticsRange,
  CompanyActivityAnalytics,
  MyActivityAnalytics,
  PendingAnalytics,
} from "@/lib/analytics/types";

type AnalyticsCardKind = "company" | "personal" | "pending";
type DataState<T> = { data: T | null; isLoading: boolean; error: string | null; retry: () => void };

const ranges: AnalyticsRange[] = ["7d", "30d", "90d"];
const departmentColors = ["#2563eb", "#0f766e", "#c2410c", "#7c3aed", "#be123c"];
const chartColors = ["#2563eb", "#0f766e", "#c2410c", "#7c3aed", "#be123c", "#64748b"];

function CountUp({ value }: { value: number }) {
  const [shown, setShown] = useState(0);

  useEffect(() => {
    let frame = 0;
    let startTime = 0;
    const duration = typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 1 : 650;
    const requestFrame = (callback: FrameRequestCallback) => typeof window.requestAnimationFrame === "function"
      ? window.requestAnimationFrame(callback)
      : window.setTimeout(() => callback(Date.now()), 16);
    const cancelFrame = (id: number) => typeof window.cancelAnimationFrame === "function"
      ? window.cancelAnimationFrame(id)
      : window.clearTimeout(id);
    const animate = (time: number) => {
      if (!startTime) startTime = time;
      const progress = Math.min(1, (time - startTime) / duration);
      setShown(Math.round(value * progress));
      if (progress < 1) frame = requestFrame(animate);
    };
    frame = requestFrame(animate);
    return () => cancelFrame(frame);
  }, [value]);

  return <>{shown}</>;
}

function RangeSwitcher({
  value,
  onChange,
  label,
}: {
  value: AnalyticsRange;
  onChange: (range: AnalyticsRange) => void;
  label: string;
}) {
  return (
    <div className="analytics-range-switcher" role="group" aria-label={label}>
      {ranges.map((range) => (
        <button
          type="button"
          key={range}
          aria-pressed={value === range}
          className={value === range ? "is-selected" : ""}
          onClick={() => onChange(range)}
        >
          {range}
        </button>
      ))}
    </div>
  );
}

function ChartPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="analytics-chart-panel">
      <h3>{title}</h3>
      {children}
    </section>
  );
}

function AnalyticsSkeleton({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`analytics-skeleton${compact ? " is-compact" : ""}`} role="status" aria-label="Loading analytics" aria-busy="true">
      <span className="skeleton w-2/3" />
      <span className="skeleton w-1/3" />
      {!compact ? <><span className="skeleton h-32 w-full" /><span className="skeleton w-4/5" /></> : null}
    </div>
  );
}

function AnalyticsError({ message, retry }: { message: string; retry: () => void }) {
  return (
    <div className="analytics-error" role="alert">
      <p>{message}</p>
      <button type="button" className="button-secondary" onClick={retry}>Try again</button>
    </div>
  );
}

function EmptyAnalytics({ children }: { children: string }) {
  return <p className="analytics-empty">{children}</p>;
}

function Sparkline({ values, label }: { values: number[]; label: string }) {
  const max = Math.max(1, ...values);
  const points = values.map((value, index) => {
    const x = values.length < 2 ? 0 : (index / (values.length - 1)) * 100;
    const y = 28 - (value / max) * 24;
    return `${x},${y}`;
  }).join(" ");
  return (
    <svg className="analytics-sparkline" viewBox="0 0 100 32" role="img" aria-label={label}>
      <title>{label}</title>
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="2.5" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

function LineChart({
  values,
  label,
  pointLabels,
  color = departmentColors[0],
}: {
  values: number[];
  label: string;
  pointLabels?: string[];
  color?: string;
}) {
  const max = Math.max(1, ...values);
  const points = values.map((value, index) => {
    const x = values.length < 2 ? 0 : (index / (values.length - 1)) * 700;
    const y = 184 - (value / max) * 160;
    return `${x},${y}`;
  }).join(" ");

  return (
    <svg className="analytics-line-chart" viewBox="0 0 700 210" role="img" aria-label={label} preserveAspectRatio="none">
      <title>{label}</title>
      {[0, 1, 2, 3].map((line) => <line key={line} x1="0" x2="700" y1={24 + line * 52} y2={24 + line * 52} className="analytics-grid-line" />)}
      <polyline points={points} fill="none" stroke={color} strokeWidth="3" vectorEffect="non-scaling-stroke" className="analytics-chart-line" />
      {values.map((value, index) => {
        const x = values.length < 2 ? 0 : (index / (values.length - 1)) * 700;
        const y = 184 - (value / max) * 160;
        return <circle key={`${index}-${value}`} cx={x} cy={y} r="4" fill={color}><title>{`${pointLabels?.[index] ?? "Period"}: ${value} activities`}</title></circle>;
      })}
    </svg>
  );
}

function MetricTile({
  title,
  onClick,
  children,
}: {
  title: string;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className="analytics-tile" onClick={onClick} aria-haspopup="dialog">
      <span className="analytics-tile-heading">
        <span className="analytics-tile-title">{title}</span>
      </span>
      {children}
      <span className="analytics-tile-hint">Open analytics <span aria-hidden="true">→</span></span>
    </button>
  );
}

function TileFailure({ state }: { state: DataState<unknown> }) {
  if (!state.error) return null;
  return <AnalyticsError message={state.error} retry={state.retry} />;
}

function CompanyTile({ state, onClick }: { state: DataState<CompanyActivityAnalytics>; onClick: (event: MouseEvent<HTMLButtonElement>) => void }) {
  const data = state.data;
  if (state.isLoading && !data) return <div className="analytics-tile analytics-tile-loading"><AnalyticsSkeleton compact /></div>;
  if ((state.error && !data) || !data) return <TileFailure state={state} />;
  const values = data.timeline.map((point) => point.departments.reduce((sum, item) => sum + item.value, 0));
  return (
    <MetricTile title="Department activity" onClick={onClick}>
      <span className="analytics-tile-main">
        <strong><CountUp value={data.totalActivities} /></strong>
        {data.totalActivities > 0 ? <Sparkline values={values} label="Company activity trend" /> : <span className="analytics-tile-unit">No activity yet</span>}
      </span>
      <span className="analytics-tile-subline"><span>activities · {data.range}</span><span className="analytics-positive">↑ {data.changePercent}%</span></span>
    </MetricTile>
  );
}

function ActivityScoreRing({ score }: { score: number }) {
  const circumference = 2 * Math.PI * 27;
  return (
    <svg className="analytics-score-ring" viewBox="0 0 64 64" role="img" aria-label={`Activity score ${score} out of 100`}>
      <circle cx="32" cy="32" r="27" className="analytics-ring-track" />
      <circle
        cx="32"
        cy="32"
        r="27"
        className="analytics-ring-value"
        strokeDasharray={circumference}
        strokeDashoffset={circumference * (1 - score / 100)}
        transform="rotate(-90 32 32)"
      />
      <text x="32" y="36" textAnchor="middle"><CountUp value={score} /></text>
    </svg>
  );
}

function PersonalTile({ state, onClick }: { state: DataState<MyActivityAnalytics>; onClick: (event: MouseEvent<HTMLButtonElement>) => void }) {
  const data = state.data;
  if (state.isLoading && !data) return <div className="analytics-tile analytics-tile-loading"><AnalyticsSkeleton compact /></div>;
  if ((state.error && !data) || !data) return <TileFailure state={state} />;
  return (
    <MetricTile title="My activity" onClick={onClick}>
      <span className="analytics-personal-tile">
        <ActivityScoreRing score={data.activityScore} />
        <span><strong>{data.activityScore === 0 ? "No activity yet" : data.scoreLabel}</strong><small>Activity score</small></span>
      </span>
      <span className="analytics-tile-subline"><span>{data.range} · compared with last period</span><span className="analytics-positive">↑ {data.changePercent}%</span></span>
    </MetricTile>
  );
}

function PendingTile({ state, onClick }: { state: DataState<PendingAnalytics>; onClick: (event: MouseEvent<HTMLButtonElement>) => void }) {
  const data = state.data;
  if (state.isLoading && !data) return <div className="analytics-tile analytics-tile-loading"><AnalyticsSkeleton compact /></div>;
  if ((state.error && !data) || !data) return <TileFailure state={state} />;
  const max = Math.max(1, data.totalPending);
  return (
    <MetricTile title="Pending analysis" onClick={onClick}>
      {data.totalPending === 0 ? (
        <span className="analytics-tile-main"><strong>0</strong><span>All caught up</span></span>
      ) : (
        <>
          <span className="analytics-tile-main"><strong><CountUp value={data.totalPending} /></strong><span className="analytics-tile-unit">items waiting</span></span>
          <span
            className="analytics-segmented-bar"
            role="img"
            aria-label={data.byType.map((item) => `${item.name}: ${item.count}`).join(", ")}
          >
            {data.byType.map((item, index) => (
              <span key={item.name} title={`${item.name}: ${item.count}`} style={{ width: `${(item.count / max) * 100}%`, background: chartColors[index % chartColors.length] }} />
            ))}
          </span>
        </>
      )}
      <span className="analytics-tile-subline"><span>Waiting for action</span><span>{data.byType.length} categories</span></span>
    </MetricTile>
  );
}

function SectionHeading({ title, range, onRangeChange }: { title: string; range: AnalyticsRange; onRangeChange: (range: AnalyticsRange) => void }) {
  return (
    <div className="analytics-detail-heading">
      <div><h2>{title}</h2><p>Insights for the selected period</p></div>
      <RangeSwitcher value={range} onChange={onRangeChange} label={`${title} date range`} />
    </div>
  );
}

function DepartmentActivityView({ data }: { data: CompanyActivityAnalytics }) {
  const [visibleDepartments, setVisibleDepartments] = useState<string[]>(data.departments.map((item) => item.name));
  const max = Math.max(1, ...data.departments.map((item) => item.activities));
  const selectedSeries = data.departments.filter((department) => visibleDepartments.includes(department.name));
  const seriesMax = Math.max(1, ...data.timeline.flatMap((point) => point.departments
    .filter((item) => visibleDepartments.includes(item.name))
    .map((item) => item.value)));
  const activitySum = data.activityTypes.reduce((sum, item) => sum + item.count, 0);

  function toggleDepartment(name: string) {
    setVisibleDepartments((current) => current.includes(name)
      ? current.filter((item) => item !== name)
      : [...current, name]);
  }

  return (
    <>
      {data.totalActivities === 0 ? <EmptyAnalytics>No department activity has been recorded for this period.</EmptyAnalytics> : null}
      <div className="analytics-detail-metric">
        <strong><CountUp value={data.totalActivities} /></strong>
        <span>company activities · <b>↑ {data.changePercent}%</b> vs previous period</span>
      </div>
      <div className="analytics-detail-grid">
        <ChartPanel title="Activity over time">
          <div className="analytics-legend">
            {data.departments.map((department, index) => (
              <button
                type="button"
                key={department.name}
                aria-pressed={visibleDepartments.includes(department.name)}
                className={visibleDepartments.includes(department.name) ? "is-selected" : ""}
                onClick={() => toggleDepartment(department.name)}
              >
                <span style={{ background: departmentColors[index % departmentColors.length] }} />
                {department.name}
              </button>
            ))}
          </div>
          <svg className="analytics-line-chart" viewBox="0 0 700 210" role="img" aria-label="Activity over time by department" preserveAspectRatio="none">
            <title>Activity over time by department</title>
            {[0, 1, 2, 3].map((line) => <line key={line} x1="0" x2="700" y1={24 + line * 52} y2={24 + line * 52} className="analytics-grid-line" />)}
            {selectedSeries.map((department) => {
              const departmentIndex = data.departments.indexOf(department);
              const points = data.timeline.map((point, index) => {
                const itemValue = point.departments.find((item) => item.name === department.name)?.value ?? 0;
                return `${(index / Math.max(1, data.timeline.length - 1)) * 700},${184 - (itemValue / seriesMax) * 160}`;
              }).join(" ");
              return (
                <polyline key={department.name} points={points} fill="none" stroke={departmentColors[departmentIndex % departmentColors.length]} strokeWidth="2.5" vectorEffect="non-scaling-stroke">
                  <title>{department.name} activity</title>
                </polyline>
              );
            })}
          </svg>
          <div className="analytics-chart-axis"><span>{data.timeline[0]?.label}</span><span>{data.timeline.at(-1)?.label}</span></div>
        </ChartPanel>
        <ChartPanel title="Department ranking">
          <div className="analytics-bar-list">
            {[...data.departments].sort((a, b) => b.activities - a.activities).map((department, index) => (
              <div className="analytics-bar-row" key={department.name}>
                <span className="analytics-bar-label">{department.name}</span>
                <span className="analytics-bar-track"><span style={{ width: `${(department.activities / max) * 100}%`, background: departmentColors[index % departmentColors.length] }} /></span>
                <strong>{department.activities}</strong>
              </div>
            ))}
          </div>
        </ChartPanel>
        <ChartPanel title="Activity types">
          <div className="analytics-type-stack" role="img" aria-label={data.activityTypes.map((item) => `${item.name}: ${item.count}`).join(", ")}>
            {data.activityTypes.map((item, index) => (
              <span key={item.name} title={`${item.name}: ${item.count}`} style={{ width: `${(item.count / Math.max(1, activitySum)) * 100}%`, background: chartColors[index % chartColors.length] }} />
            ))}
          </div>
          <ul className="analytics-legend-list">
            {data.activityTypes.map((item, index) => (
              <li key={item.name}><span style={{ background: chartColors[index % chartColors.length] }} />{item.name}<strong>{item.count}</strong></li>
            ))}
          </ul>
        </ChartPanel>
        <ChartPanel title="Busiest days and hours">
          <div className="analytics-heatmap" role="img" aria-label="Activity heatmap by day and working hours">
            <span className="analytics-heatmap-spacer" />
            {["8a", "10a", "12p", "2p", "4p", "6p", "8p", "10p"].map((hour) => <span className="analytics-heatmap-label" key={hour}>{hour}</span>)}
            {data.heatmap.map((row) => (
              <div className="analytics-heatmap-row" key={row.day}>
                <span>{row.day}</span>
                {row.hours.map((count, index) => <i key={`${row.day}-${index}`} title={`${row.day}, ${count} activities`} style={{ opacity: 0.12 + Math.min(1, count / 10) * 0.78 }} />)}
              </div>
            ))}
          </div>
        </ChartPanel>
        <ChartPanel title="Most and least active">
          <div className="analytics-trend-lists">
            <div><h4>Top departments</h4>{[...data.departments].sort((a, b) => b.activities - a.activities).slice(0, 5).map((item) => <p key={item.name}><span>{item.name}</span><strong>{item.trend === "up" ? "↑" : item.trend === "down" ? "↓" : "→"} {item.activities}</strong></p>)}</div>
            <div><h4>Needs a boost</h4>{[...data.departments].sort((a, b) => a.activities - b.activities).slice(0, 5).map((item) => <p key={item.name}><span>{item.name}</span><strong>{item.trend === "up" ? "↑" : item.trend === "down" ? "↓" : "→"} {item.activities}</strong></p>)}</div>
          </div>
        </ChartPanel>
      </div>
      <ChartPanel title="Department activity detail">
        <div className="analytics-table-frame">
          <table className="analytics-table">
            <thead><tr><th>Department</th><th>Activities</th><th>Tasks completed</th><th>Completion rate</th><th>Active staff</th></tr></thead>
            <tbody>{data.departments.map((item) => (
              <tr key={item.name}>
                <td data-label="Department">{item.name}</td>
                <td data-label="Activities">{item.activities}</td>
                <td data-label="Tasks completed">{item.tasksCompleted}</td>
                <td data-label="Completion rate">{item.completionRate}%</td>
                <td data-label="Active staff">{item.activeStaff}</td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </ChartPanel>
    </>
  );
}

function PersonalActivityView({ data }: { data: MyActivityAnalytics }) {
  const totalTasks = data.taskBreakdown.reduce((sum, item) => sum + item.count, 0);
  const taskColors = ["#0f766e", "#2563eb", "#c2410c", "#64748b"];
  return (
    <>
      {data.activityScore === 0 && data.dailyTrend.every((point) => point.value === 0)
        ? <EmptyAnalytics>No personal activity has been recorded for this period.</EmptyAnalytics>
        : null}
      <div className="analytics-personal-summary">
        <ActivityScoreRing score={data.activityScore} />
        <div><strong>{data.scoreLabel}</strong><p>Activity score: {data.activityScore}/100 · {data.changePercent}% higher than the previous period</p></div>
      </div>
      <ChartPanel title="How your activity score is calculated">
        <div className="analytics-factor-list">
          {data.scoreFactors.map((factor) => (
            <div className="analytics-factor" key={factor.name}>
              <div><strong>{factor.name}</strong><span>{factor.detail}</span></div>
              <span className="analytics-factor-track"><span style={{ width: `${Math.min(100, (factor.value / factor.maximum) * 100)}%` }} /></span>
            </div>
          ))}
        </div>
        <p className="analytics-method-note">The score combines task completion, on-time delivery, active days, and response speed. Each factor is normalized before aggregation.</p>
      </ChartPanel>
      <div className="analytics-detail-grid">
        <ChartPanel title="Activity trend">
          <LineChart
            values={data.dailyTrend.map((point) => point.value)}
            pointLabels={data.dailyTrend.map((point) => point.label)}
            label="Daily personal activity trend"
          />
          <div className="analytics-chart-axis"><span>{data.dailyTrend[0]?.label}</span><span>{data.dailyTrend.at(-1)?.label}</span></div>
        </ChartPanel>
        <ChartPanel title="Task breakdown">
          <div className="analytics-donut-layout">
            <div className="analytics-donut" role="img" aria-label={data.taskBreakdown.map((item) => `${item.name}: ${item.count}`).join(", ")} style={{ background: `conic-gradient(${data.taskBreakdown.map((item, index) => {
              const previous = data.taskBreakdown.slice(0, index).reduce((sum, value) => sum + value.count, 0);
              const start = (previous / Math.max(1, totalTasks)) * 100;
              const end = ((previous + item.count) / Math.max(1, totalTasks)) * 100;
              return `${taskColors[index % taskColors.length]} ${start}% ${end}%`;
            }).join(", ")})` }}><span>{totalTasks}<small>tasks</small></span></div>
            <ul className="analytics-legend-list">{data.taskBreakdown.map((item, index) => <li key={item.name}><span style={{ background: taskColors[index % taskColors.length] }} />{item.name}<strong>{item.count}</strong></li>)}</ul>
          </div>
        </ChartPanel>
        <ChartPanel title="You vs department average">
          <div className="analytics-comparison">
            <div><span>You</span><span className="analytics-comparison-track"><span style={{ width: `${data.departmentComparison.user}%` }} /></span><strong>{data.departmentComparison.user}</strong></div>
            <div><span>Department average</span><span className="analytics-comparison-track is-average"><span style={{ width: `${data.departmentComparison.departmentAverage}%` }} /></span><strong>{data.departmentComparison.departmentAverage}</strong></div>
          </div>
        </ChartPanel>
        <ChartPanel title="Recent achievements">
          <ul className="analytics-achievements">{data.achievements.map((item) => <li key={item.title}><strong>{item.title}</strong><span>{item.detail}</span><time dateTime={item.date}>{item.date}</time></li>)}</ul>
        </ChartPanel>
      </div>
      <ChartPanel title="Your last 12 weeks">
        <div className="analytics-contribution-calendar" role="img" aria-label="Contribution calendar for the last twelve weeks">
          {data.contributionWeeks.map((week) => (
            <div className="analytics-calendar-week" key={week.week} aria-label={week.week}>
              {week.days.map((day) => <span key={day.date} title={`${day.date}: ${day.count} activities`} aria-label={`${day.date}: ${day.count} activities`} data-level={Math.min(4, day.count)} />)}
            </div>
          ))}
        </div>
        <div className="analytics-calendar-caption"><span>Less activity</span><span className="analytics-calendar-scale">{[0, 1, 2, 3, 4].map((level) => <i key={level} data-level={level} />)}</span><span>More activity</span></div>
      </ChartPanel>
    </>
  );
}

function PendingAnalysisView({ data }: { data: PendingAnalytics }) {
  const maxAging = Math.max(1, ...data.aging.map((item) => item.count));
  const maxDepartment = Math.max(1, ...data.departments.map((item) => item.count));
  return (
    <>
      {data.totalPending === 0 ? <EmptyAnalytics>All caught up — there are no pending items that need attention.</EmptyAnalytics> : null}
      <div className="analytics-detail-metric">
        <strong><CountUp value={data.totalPending} /></strong>
        <span>{data.totalPending === 1 ? "item needs" : "items need"} attention in this period</span>
      </div>
      <div className="analytics-detail-grid">
        <ChartPanel title="Pending by type">
          {data.totalPending === 0 ? <EmptyAnalytics>Nothing is waiting for action.</EmptyAnalytics> : (
            <ul className="analytics-pending-types">{data.byType.map((item, index) => (
              <li key={item.name}>
                <Link href={item.href}><span className="analytics-type-dot" style={{ background: chartColors[index % chartColors.length] }} />{item.name}<strong>{item.count}</strong></Link>
              </li>
            ))}</ul>
          )}
        </ChartPanel>
        <ChartPanel title="Aging breakdown">
          <div className="analytics-bar-list">{data.aging.map((item) => (
            <div className="analytics-bar-row" key={item.label}>
              <span className="analytics-bar-label">{item.label}{item.label === "30+ days" && item.count ? " · oldest" : ""}</span>
              <span className="analytics-bar-track"><span className={item.label === "30+ days" ? "is-oldest" : ""} style={{ width: `${(item.count / maxAging) * 100}%` }} /></span>
              <strong>{item.count}</strong>
            </div>
          ))}</div>
        </ChartPanel>
        <ChartPanel title="Pending by department">
          <div className="analytics-bar-list">{data.departments.map((item, index) => (
            <div className="analytics-bar-row" key={item.name}>
              <span className="analytics-bar-label">{item.name}</span>
              <span className="analytics-bar-track"><span style={{ width: `${(item.count / maxDepartment) * 100}%`, background: departmentColors[index % departmentColors.length] }} /></span>
              <strong>{item.count}</strong>
            </div>
          ))}</div>
        </ChartPanel>
      </div>
      <ChartPanel title="Needs attention">
        {data.topItems.length ? (
          <ul className="analytics-attention-list">{data.topItems.map((item) => (
            <li key={item.title}>
              <Link href={item.href}><span><strong>{item.title}</strong><small>{item.type}</small></span><span className={item.ageDays >= 8 ? "is-overdue" : ""}>{item.ageDays}d waiting <span aria-hidden="true">→</span></span></Link>
            </li>
          ))}</ul>
        ) : <EmptyAnalytics>All caught up — there are no items needing attention.</EmptyAnalytics>}
      </ChartPanel>
    </>
  );
}

export function DashboardAnalytics() {
  const activeCardRef = useRef<HTMLButtonElement>(null);
  const [companyRange, setCompanyRange] = useState<AnalyticsRange>("30d");
  const [personalRange, setPersonalRange] = useState<AnalyticsRange>("30d");
  const [pendingRange, setPendingRange] = useState<AnalyticsRange>("30d");
  const [openedCard, setOpenedCard] = useState<AnalyticsCardKind | null>(null);
  const company = useCompanyActivity(companyRange);
  const personal = useMyActivity(personalRange);
  const pending = usePending(pendingRange);

  const title = openedCard === "company" ? "Department activity"
    : openedCard === "personal" ? "My activity"
      : "Pending analysis";
  const selectedRange = openedCard === "company" ? companyRange : openedCard === "personal" ? personalRange : pendingRange;
  const selectedState = openedCard === "company" ? company : openedCard === "personal" ? personal : pending;
  const setSelectedRange = openedCard === "company" ? setCompanyRange : openedCard === "personal" ? setPersonalRange : setPendingRange;

  function openAnalytics(kind: AnalyticsCardKind, event: MouseEvent<HTMLButtonElement>) {
    activeCardRef.current = event.currentTarget;
    setOpenedCard(kind);
  }

  return (
    <>
      <CompanyTile state={company} onClick={(event) => openAnalytics("company", event)} />
      <PersonalTile state={personal} onClick={(event) => openAnalytics("personal", event)} />
      <PendingTile state={pending} onClick={(event) => openAnalytics("pending", event)} />
      <GlassModal
        open={openedCard !== null}
        title={title}
        subtitle="Dashboard analytics"
        onClose={() => setOpenedCard(null)}
        dialogClassName="dashboard-analytics-dialog"
        panelClassName="dashboard-analytics-glass"
        returnFocusRef={activeCardRef}
      >
        {openedCard ? (
          <>
            <SectionHeading title={title} range={selectedRange} onRangeChange={setSelectedRange} />
            {selectedState.isLoading ? <AnalyticsSkeleton /> : selectedState.error ? (
              <AnalyticsError message={selectedState.error} retry={selectedState.retry} />
            ) : !selectedState.data ? (
                <EmptyAnalytics>Analytics data is not available for this period.</EmptyAnalytics>
            ) : openedCard === "company" ? (
                company.data ? <DepartmentActivityView data={company.data} /> : <EmptyAnalytics>Analytics data is not available for this period.</EmptyAnalytics>
            ) : openedCard === "personal" ? (
                personal.data ? <PersonalActivityView data={personal.data} /> : <EmptyAnalytics>Analytics data is not available for this period.</EmptyAnalytics>
            ) : (
                pending.data ? <PendingAnalysisView data={pending.data} /> : <EmptyAnalytics>Analytics data is not available for this period.</EmptyAnalytics>
            )}
          </>
        ) : null}
      </GlassModal>
    </>
  );
}
