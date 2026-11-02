import { EmptyState, PageHeader, Panel } from "@/components/workspace/ui";

export default function SettingsPage() {
  return (
    <>
      <PageHeader title="Settings" description="Configure workspace preferences." />
      <Panel title="Workspace settings">
        <EmptyState>No configurable settings are available yet.</EmptyState>
      </Panel>
    </>
  );
}
