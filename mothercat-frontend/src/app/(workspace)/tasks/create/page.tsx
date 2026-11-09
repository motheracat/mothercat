import { CreateTaskForm } from "@/components/workspace/create-task-form";
import { isDevOpen } from "@/lib/auth/mode";
import { getCurrentSession } from "@/lib/auth/session";
import { hasPermission } from "@/lib/auth/permissions";

export default async function CreateTaskPage() {
  const session = await getCurrentSession();
  const devOpen = isDevOpen();
  return (
    <CreateTaskForm
      canCreate={!devOpen && hasPermission(session, "tasks.create")}
      canAssign={!devOpen && hasPermission(session, "tasks.assign")}
      previewMode={devOpen}
    />
  );
}
