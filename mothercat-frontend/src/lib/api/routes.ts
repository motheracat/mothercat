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
  },
  dashboard: "/dashboard",
  departments: "/departments",
  roles: "/roles",
  audit: "/audit",
  settings: "/settings",
} as const;

export type ApiRoute = (typeof apiRoutes)[keyof typeof apiRoutes];
