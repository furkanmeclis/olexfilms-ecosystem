import { notificationsService } from "@/features/notifications/services/notifications.service";
import { rolesService } from "@/features/roles/services/roles.service";
import { usersService } from "@/features/users/services/users.service";
import { OVERVIEW_LIST_LIMIT } from "@/features/platform-overview/constants";

export type PlatformOverviewStats = {
  rolesTotal?: number;
  usersTotal?: number;
  notificationsTotal?: number;
};

export type OverviewFetchScopes = {
  roles?: boolean;
  users?: boolean;
  notifications?: boolean;
};

/**
 * Light aggregates from existing list endpoints (`limit=1` + `total`).
 */
export const overviewService = {
  async getStats(
    scopes: OverviewFetchScopes = {
      roles: true,
      users: true,
      notifications: true,
    },
  ): Promise<PlatformOverviewStats> {
    const stats: PlatformOverviewStats = {};
    const tasks: Promise<void>[] = [];

    if (scopes.roles) {
      tasks.push(
        rolesService
          .list({ limit: OVERVIEW_LIST_LIMIT, offset: 0 })
          .then((page) => {
            stats.rolesTotal = page.total;
          }),
      );
    }

    if (scopes.users) {
      tasks.push(
        usersService
          .list({ limit: OVERVIEW_LIST_LIMIT, offset: 0 })
          .then((page) => {
            stats.usersTotal = page.total;
          }),
      );
    }

    if (scopes.notifications) {
      tasks.push(
        notificationsService
          .listPlatform({ limit: OVERVIEW_LIST_LIMIT, offset: 0 })
          .then((page) => {
            stats.notificationsTotal = page.total;
          }),
      );
    }

    await Promise.all(tasks);
    return stats;
  },
};
