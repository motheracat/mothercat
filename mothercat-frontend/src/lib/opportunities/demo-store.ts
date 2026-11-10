export type OpportunityType = "job" | "siwes";
export type OpportunityStatus = "draft" | "published";

export type Opportunity = {
  id: string;
  type: OpportunityType;
  title: string;
  location: string;
  workMode: string;
  summary: string;
  description: string;
  requirements: string;
  deadline: string;
  status: OpportunityStatus;
  createdAt: string;
};

export type OpportunityApplication = {
  id: string;
  opportunityId: string;
  opportunityTitle: string;
  type: OpportunityType;
  fullName: string;
  email: string;
  phone: string;
  institution: string;
  course: string;
  matricNumber: string;
  message: string;
  fileName: string;
  fileType: string;
  fileSize: number;
  submittedAt: string;
};

export type OpportunityDemoData = {
  opportunities: Opportunity[];
  applications: OpportunityApplication[];
};

export const opportunityStoreKey = "mothercat:opportunities-demo:v1";
export const opportunityStoreEvent = "mothercat:opportunities-demo-updated";

const initialDemoData: OpportunityDemoData = {
  opportunities: [
    {
      id: "demo-job-civil-engineer",
      type: "job",
      title: "Graduate Civil Engineer",
      location: "Lagos, Nigeria",
      workMode: "Full-time",
      summary: "Join project teams delivering resilient infrastructure and construction projects.",
      description: "Work alongside experienced engineers on planning, site coordination, and project delivery.",
      requirements: "Degree in Civil Engineering; strong communication and analytical skills.",
      deadline: "2027-01-31",
      status: "published",
      createdAt: "2026-11-01T09:00:00.000Z",
    },
    {
      id: "demo-siwes-placement",
      type: "siwes",
      title: "SIWES Industrial Training Placement",
      location: "Lagos, Nigeria",
      workMode: "On-site",
      summary: "A supervised industrial training placement for students in engineering and built-environment programmes.",
      description: "Students will gain practical exposure to ongoing projects, workplace safety, and professional engineering teams.",
      requirements: "Currently enrolled in a relevant programme and able to provide an institutional SIWES introduction letter.",
      deadline: "2027-02-28",
      status: "published",
      createdAt: "2026-11-01T09:00:00.000Z",
    },
  ],
  applications: [],
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isOpportunity(value: unknown): value is Opportunity {
  return isRecord(value) &&
    typeof value.id === "string" &&
    (value.type === "job" || value.type === "siwes") &&
    typeof value.title === "string" &&
    typeof value.location === "string" &&
    typeof value.workMode === "string" &&
    typeof value.summary === "string" &&
    typeof value.description === "string" &&
    typeof value.requirements === "string" &&
    typeof value.deadline === "string" &&
    (value.status === "draft" || value.status === "published") &&
    typeof value.createdAt === "string";
}

function isApplication(value: unknown): value is OpportunityApplication {
  return isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.opportunityId === "string" &&
    typeof value.opportunityTitle === "string" &&
    (value.type === "job" || value.type === "siwes") &&
    typeof value.fullName === "string" &&
    typeof value.email === "string" &&
    typeof value.phone === "string" &&
    typeof value.institution === "string" &&
    typeof value.course === "string" &&
    typeof value.matricNumber === "string" &&
    typeof value.message === "string" &&
    typeof value.fileName === "string" &&
    typeof value.fileType === "string" &&
    typeof value.fileSize === "number" &&
    typeof value.submittedAt === "string";
}

function parseDemoData(value: unknown): OpportunityDemoData {
  if (
    !isRecord(value) ||
    !Array.isArray(value.opportunities) ||
    !value.opportunities.every(isOpportunity) ||
    !Array.isArray(value.applications) ||
    !value.applications.every(isApplication)
  ) {
    throw new Error("Saved opportunity demo data is invalid. Clear this browser's opportunity demo data and reload.");
  }
  return { opportunities: value.opportunities, applications: value.applications };
}

export function readOpportunityDemoData(): OpportunityDemoData {
  const saved = window.localStorage.getItem(opportunityStoreKey);
  if (saved === null) {
    const seed = structuredClone(initialDemoData);
    writeOpportunityDemoData(seed);
    return seed;
  }
  return parseDemoData(JSON.parse(saved));
}

export function writeOpportunityDemoData(data: OpportunityDemoData): void {
  window.localStorage.setItem(opportunityStoreKey, JSON.stringify(data));
  window.dispatchEvent(new CustomEvent(opportunityStoreEvent));
}

export function createDemoOpportunity(
  input: Omit<Opportunity, "id" | "createdAt">,
): Opportunity {
  return {
    ...input,
    id: `demo-${crypto.randomUUID()}`,
    createdAt: new Date().toISOString(),
  };
}

export function createDemoApplication(
  input: Omit<OpportunityApplication, "id" | "submittedAt">,
): OpportunityApplication {
  return {
    ...input,
    id: `demo-${crypto.randomUUID()}`,
    submittedAt: new Date().toISOString(),
  };
}
