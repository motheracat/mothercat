export const apiRoutes = {
  auth: {
    session: "/auth/session",
    login: "/auth/login",
    logout: "/auth/logout",
    forgotPassword: "/auth/forgot-password",
    resetPassword: "/auth/reset-password",
    activate: "/auth/activate",
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
