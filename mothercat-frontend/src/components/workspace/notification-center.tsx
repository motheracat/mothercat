"use client";

import Link from "next/link";
import { Bell, Check, CheckCheck, RefreshCw, Settings2 } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import { EmptyState, ErrorBanner, PageHeader } from "@/components/workspace/ui";
import {
  formatNotificationError,
  getUnreadNotificationCount,
  getNotificationPreferences,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  notificationEventTypes,
  replaceNotificationPreferences,
  type AppNotification,
  type NotificationEventType,
  type NotificationPreference,
  notificationsUpdatedEvent,
} from "@/lib/notifications/api";

const pageSize = 20;

const eventLabels: Record<NotificationEventType, string> = {
  "task.assigned": "Task assigned to you",
  "task.submitted": "Task submitted for review",
  "task.approved": "Task approved",
  "task.changes_requested": "Changes requested",
  "task.rejected": "Task rejected",
  "task.comment_added": "Comment added to a task",
  "task.due_soon": "Task due soon",
  "task.overdue": "Task overdue",
};

function notificationTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Unknown date" : date.toLocaleString();
}

export function NotificationCenter() {
  const [notifications, setNotifications] = useState<AppNotification[]>([]);
  const [preferences, setPreferences] = useState<NotificationPreference[]>([]);
  const [total, setTotal] = useState(0);
  const [unreadCount, setUnreadCount] = useState(0);
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [preferencesLoading, setPreferencesLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [preferenceError, setPreferenceError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [preferencesDirty, setPreferencesDirty] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const unreadCountRef = useRef(0);

  const reload = useCallback(() => {
    setError(null);
    setPreferenceError(null);
    setLoading(true);
    setPreferencesLoading(true);
    setReloadKey((key) => key + 1);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void Promise.allSettled([
      listNotifications({ unread: unreadOnly, limit: pageSize, offset }, controller.signal),
      getUnreadNotificationCount(controller.signal),
    ])
      .then(([listResult, countResult]) => {
        if (controller.signal.aborted) return;
        const failures: string[] = [];
        if (listResult.status === "fulfilled") {
          setNotifications(listResult.value.notifications);
          setTotal(listResult.value.page.total);
        } else {
          setNotifications([]);
          setTotal(0);
          failures.push(formatNotificationError(listResult.reason, "Unable to load notifications."));
        }
        if (countResult.status === "fulfilled") {
          unreadCountRef.current = countResult.value;
          setUnreadCount(countResult.value);
        } else {
          failures.push(formatNotificationError(countResult.reason, "Unable to load unread count."));
        }
        setError(failures.length ? failures.join(" ") : null);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [unreadOnly, offset, reloadKey]);

  useEffect(() => {
    const controller = new AbortController();
    void getNotificationPreferences(controller.signal)
      .then((nextPreferences) => {
        setPreferences(nextPreferences);
        setPreferencesDirty(false);
        setPreferenceError(null);
      })
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) {
          setPreferenceError(formatNotificationError(loadError, "Unable to load notification preferences."));
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setPreferencesLoading(false);
      });
    return () => controller.abort();
  }, [reloadKey]);

  async function markRead(notification: AppNotification) {
    if (notification.read_at) return;
    setError(null);
    try {
      await markNotificationRead(notification.id);
      setNotifications((current) => current.map((item) => item.id === notification.id
        ? { ...item, read_at: new Date().toISOString() }
        : item));
      const nextUnreadCount = Math.max(0, unreadCountRef.current - 1);
      unreadCountRef.current = nextUnreadCount;
      setUnreadCount(nextUnreadCount);
      window.dispatchEvent(new CustomEvent(notificationsUpdatedEvent, { detail: nextUnreadCount }));
    } catch (requestError) {
      setError(formatNotificationError(requestError, "Unable to mark this notification as read."));
    }
  }

  async function markAllRead() {
    setSaving(true);
    setError(null);
    try {
      const markedRead = await markAllNotificationsRead();
      const readAt = new Date().toISOString();
      setNotifications((current) => current.map((item) => ({ ...item, read_at: item.read_at ?? readAt })));
      const nextUnreadCount = Math.max(0, unreadCountRef.current - markedRead);
      unreadCountRef.current = nextUnreadCount;
      setUnreadCount(nextUnreadCount);
      setNotice("All notifications marked as read.");
      window.dispatchEvent(new CustomEvent(notificationsUpdatedEvent, { detail: nextUnreadCount }));
    } catch (requestError) {
      setError(formatNotificationError(requestError, "Unable to mark notifications as read."));
    } finally {
      setSaving(false);
    }
  }

  function updatePreference(eventType: NotificationEventType, enabled: boolean) {
    setPreferencesDirty(true);
    setPreferences((current) => current.map((preference) => preference.event_type === eventType
      ? { ...preference, enabled }
      : preference));
  }

  async function savePreferences() {
    setSaving(true);
    setPreferenceError(null);
    setNotice(null);
    try {
      const saved = await replaceNotificationPreferences(preferences);
      setPreferences(saved);
      setPreferencesDirty(false);
      setNotice("Notification preferences saved.");
    } catch (requestError) {
      setPreferenceError(formatNotificationError(requestError, "Unable to save notification preferences."));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="notification-center">
      <PageHeader
        title="Notifications"
        description="Important task updates and activity for your account."
        actions={(
          <>
            <button type="button" className="button-secondary" onClick={reload} disabled={loading} aria-label="Refresh notifications">
              <RefreshCw size={16} aria-hidden="true" /> Refresh
            </button>
            <Link href="/audit" className="button-secondary">View audit log</Link>
          </>
        )}
      />

      {error ? <ErrorBanner message={error} onRetry={reload} /> : null}
      {notice ? <p role="status" className="access-notice">{notice}</p> : null}

      <section className="surface-card notification-inbox" aria-label="Notification inbox" aria-busy={loading}>
        <div className="notification-inbox-header">
          <div>
            <h2>Inbox</h2>
            <p>{unreadCount ? `${unreadCount} unread notification${unreadCount === 1 ? "" : "s"}` : "You’re all caught up."}</p>
          </div>
          <div className="notification-inbox-tools">
            <label>
              <span className="sr-only">Show notifications</span>
              <select
                className="form-select"
                value={unreadOnly ? "unread" : "all"}
                onChange={(event) => {
                  setLoading(true);
                  setUnreadOnly(event.target.value === "unread");
                  setOffset(0);
                }}
              >
                <option value="all">All notifications</option>
                <option value="unread">Unread only</option>
              </select>
            </label>
            <button type="button" className="button-secondary" disabled={!unreadCount || saving} onClick={() => void markAllRead()}>
              <CheckCheck size={16} aria-hidden="true" /> Mark all read
            </button>
          </div>
        </div>
        {loading ? (
          <p role="status" className="notification-loading">Loading notifications…</p>
        ) : notifications.length ? (
          <ul className="notification-list">
            {notifications.map((notification) => (
              <li key={notification.id} className={`notification-item${notification.read_at ? " is-read" : " is-unread"}`}>
                <span className="notification-item-icon"><Bell size={17} aria-hidden="true" /></span>
                <div className="notification-item-copy">
                  <strong>{notification.title}</strong>
                  <span className="notification-event-label">{eventLabels[notification.event_type]}</span>
                  <p>{notification.body}</p>
                  <time dateTime={notification.created_at}>{notificationTime(notification.created_at)}</time>
                </div>
                {notification.resource?.type === "task" ? (
                  <Link href="/tasks" className="button-ghost notification-resource-link">View task</Link>
                ) : null}
                {!notification.read_at ? (
                  <button type="button" className="notification-mark-read" aria-label={`Mark ${notification.title} as read`} onClick={() => void markRead(notification)}>
                    <Check size={17} aria-hidden="true" />
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState>
            {error ? "Notifications could not be loaded." : unreadOnly ? "No unread notifications." : "No notifications yet. Task updates will appear here."}
          </EmptyState>
        )}
        <div className="notification-pagination">
          <span>{total ? `Showing ${offset + 1}–${Math.min(offset + notifications.length, total)} of ${total}` : "No notifications"}</span>
          <div>
            <button type="button" className="button-secondary" disabled={loading || offset === 0} onClick={() => { setLoading(true); setOffset((value) => Math.max(0, value - pageSize)); }}>Previous</button>
            <button type="button" className="button-secondary" disabled={loading || offset + pageSize >= total} onClick={() => { setLoading(true); setOffset((value) => value + pageSize); }}>Next</button>
          </div>
        </div>
      </section>

      <section className="surface-card notification-preferences" aria-labelledby="notification-preferences-title">
        <div className="surface-card-header">
          <div>
            <h2 className="surface-title" id="notification-preferences-title"><Settings2 size={17} aria-hidden="true" /> Notification preferences</h2>
            <p>Choose which task events should notify you. Reminders are generated automatically.</p>
          </div>
          <button type="button" className="button-primary" disabled={saving || preferencesLoading || Boolean(preferenceError) || !preferencesDirty} onClick={() => void savePreferences()}>
            {saving ? "Saving…" : "Save preferences"}
          </button>
        </div>
        {preferenceError ? (
          <div className="notification-preference-error">
            <p role="alert" className="field-error">{preferenceError}</p>
            <button type="button" className="button-secondary" onClick={reload}>Retry preferences</button>
          </div>
        ) : null}
        <div className="notification-preference-list">
          {notificationEventTypes.map((eventType) => (
            <label key={eventType} className="notification-preference-option">
              <span>
                <strong>{eventLabels[eventType]}</strong>
                <small>{eventType}</small>
              </span>
              <input
                type="checkbox"
                checked={preferences.find((item) => item.event_type === eventType)?.enabled ?? true}
                onChange={(event) => updatePreference(eventType, event.target.checked)}
                aria-label={eventLabels[eventType]}
                disabled={saving || preferencesLoading || Boolean(preferenceError)}
              />
            </label>
          ))}
        </div>
      </section>
    </div>
  );
}
