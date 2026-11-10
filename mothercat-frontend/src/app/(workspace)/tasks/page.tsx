import { TaskManagement } from "@/components/workspace/task-management";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function TasksPage() {
  const session = await getCurrentSession();

  return (
    <TaskManagement
      currentUserId={session?.userId ?? ""}
      canCreate={hasPermission(session, "tasks.create")}
      canAssign={hasPermission(session, "tasks.assign")}
      canReview={hasPermission(session, "tasks.review")}
      canCancel={hasPermission(session, "tasks.cancel")}
    />
  );
}
