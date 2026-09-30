import { NextResponse } from "next/server";

import { getAppMode } from "@/config/app-mode";

export async function GET() {
  return NextResponse.json({
    success: true,
    data: {
      mode: getAppMode(),
    },
  });
}
