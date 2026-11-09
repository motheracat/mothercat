import { TaskManagement } from "@/components/workspace/task-management";
import { isDevOpen } from "@/lib/auth/mode";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function TasksPage() {
  const session = await getCurrentSession();
  const devOpen = isDevOpen();
  return (
    <TaskManagement
      currentUserId={devOpen ? "" : session?.userId ?? ""}
      canCreate={!devOpen && hasPermission(session, "tasks.create")}
      canAssign={!devOpen && hasPermission(session, "tasks.assign")}
      canReview={!devOpen && hasPermission(session, "tasks.review")}
      canCancel={!devOpen && hasPermission(session, "tasks.cancel")}
    />
  );
}
