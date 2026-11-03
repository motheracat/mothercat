export const apiRoutes = {
  auth: {
    session: "/auth/session",
    login: "/auth/login",
    logout: "/auth/logout",
    forgotPassword: "/auth/password/forgot",
    resetPassword: "/auth/password/reset",
    changePassword: "/auth/password/change",
    activate: "/auth/activation/complete",
  },
  staff: {
    list: "/staff",
    create: "/staff",
    details: (staffId: string) => `/staff/${staffId}`,
    update: (staffId: string) => `/staff/${staffId}`,
    deactivate: (staffId: string) => `/staff/${staffId}/deactivate`,
    provision: (staffId: string) => `/staff/${staffId}/provision`,
    provisionAdmin: (staffId: string) => `/staff/${staffId}/provision-admin`,
    imports: "/staff/imports",
    importDetails: (importId: string) => `/staff/imports/${importId}`,
  },
  dashboard: "/dashboard",
  departments: {
    list: "/departments",
    create: "/departments",
    details: (departmentId: string) => `/departments/${departmentId}`,
    update: (departmentId: string) => `/departments/${departmentId}`,
  },
  departmentDetails: (departmentId: string) => `/departments/${departmentId}`,
  roles: "/roles",
  audit: "/audit",
  settings: "/settings",
} as const;

export type ApiRoute = string;
