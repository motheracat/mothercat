import { CreateTaskForm } from "@/components/workspace/create-task-form";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function CreateTaskPage() {
  const session = await getCurrentSession();

  return (
    <CreateTaskForm
      canCreate={hasPermission(session, "tasks.create")}
      canAssign={hasPermission(session, "tasks.assign")}
      previewMode={false}
    />
  );
}
