export { auth as proxy } from "@/auth";

export const config = {
  matcher: [
    "/",
    "/register",
    "/t/:path*",
    "/profile/:path*",
    "/platform/:path*",
  ],
};
