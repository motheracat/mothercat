import { StaffImportDetails } from "@/components/workspace/staff-import";

export default async function StaffImportDetailsPage({
  params,
}: {
  params: Promise<{ importId: string }>;
}) {
  const { importId } = await params;
  return <StaffImportDetails importId={importId} />;
}
