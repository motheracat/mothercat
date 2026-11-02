import Link from "next/link";
import { Bell, ChevronRight } from "lucide-react";

import { PageHeader } from "@/components/workspace/ui";

export default function NotificationsPage() {
  return (
    <>
      <PageHeader
        title="Notifications"
        description="Important updates and activity from your workspace."
        actions={<Link href="/audit" className="button-secondary">View audit log</Link>}
      />

      <section aria-label="Notification inbox" className="surface-card notification-inbox">
        <div className="notification-inbox-header">
          <div>
            <h2>Inbox</h2>
            <p>Your workspace notifications will appear here.</p>
          </div>
          <span className="notification-state">
            <span aria-hidden="true" />
            All caught up
          </span>
        </div>
        <div className="notification-empty-state">
          <span className="notification-empty-icon" aria-hidden="true">
            <Bell size={20} strokeWidth={1.8} />
          </span>
          <h3>No new notifications</h3>
          <p>When there&apos;s an update that needs your attention, you&apos;ll find it here.</p>
          <Link href="/audit" className="notification-audit-link">
            Browse workspace activity <ChevronRight size={15} aria-hidden="true" />
          </Link>
        </div>
      </section>
    </>
  );
}
