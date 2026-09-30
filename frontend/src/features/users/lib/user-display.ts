import type { PublicUser } from "@/features/users/services/users.service";

export function userFullName(user: Pick<PublicUser, "name" | "surname">) {
  return `${user.name} ${user.surname}`.trim();
}

export function userInitials(user: Pick<PublicUser, "name" | "surname">) {
  return `${user.name.charAt(0)}${user.surname.charAt(0)}`.toUpperCase();
}
