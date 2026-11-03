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
    details: (staffId: string) => `/staff/${staffId}`,
    deactivate: (staffId: string) => `/staff/${staffId}/deactivate`,
    provision: (staffId: string) => `/staff/${staffId}/provision`,
    imports: "/staff/imports",
    importDetails: (importId: string) => `/staff/imports/${importId}`,
  },
  dashboard: "/dashboard",
  departments: "/departments",
  departmentDetails: (departmentId: string) => `/departments/${departmentId}`,
  roles: "/roles",
  audit: "/audit",
  settings: "/settings",
} as const;

export type ApiRoute = (typeof apiRoutes)[keyof typeof apiRoutes];
