import { describe, expect, it } from "vitest";

import {
  availableLocales,
  buildFolderTree,
  flattenFolderTree,
  folderSubtreeIds,
  isFolderNotEmptyError,
  LIBRARY_MAX_UPLOAD_BYTES,
  libraryLocale,
  parseTags,
  uploadFileError,
  versionsForLocale,
} from "@/features/library/lib/library";
import type {
  LibraryFolder,
  LibraryVersion,
} from "@/features/library/services/library.service";
import { ApiError } from "@/lib/api/errors";

function file(size: number) {
  const f = new File(["x"], "a.pdf");
  Object.defineProperty(f, "size", { value: size });
  return f;
}

function folder(uuid: string, parent: string | null, name = uuid) {
  return {
    uuid,
    parent_uuid: parent,
    name,
    sort_order: 0,
    created_at: "",
    updated_at: "",
  } satisfies LibraryFolder;
}

function v(locale: string, no: number): LibraryVersion {
  return {
    uuid: `${locale}-${no}`,
    locale,
    version_no: no,
    mime: "application/pdf",
    size_bytes: 1,
    sha256: "",
    created_at: "",
  };
}

describe("library lib", () => {
  it("rejects files over 50 MB before upload", () => {
    expect(uploadFileError(file(LIBRARY_MAX_UPLOAD_BYTES))).toBeNull();
    expect(uploadFileError(file(LIBRARY_MAX_UPLOAD_BYTES + 1))).toBe(
      "library.upload.too_large",
    );
    expect(uploadFileError(null)).toBe("library.upload.file_required");
  });

  it("filters versions by language, newest first", () => {
    const all = [v("tr", 1), v("de", 1), v("tr", 3), v("tr", 2)];
    expect(versionsForLocale(all, "tr").map((x) => x.version_no)).toEqual([
      3, 2, 1,
    ]);
    expect(versionsForLocale(all, "en")).toEqual([]);
    expect(availableLocales(all)).toEqual(["tr", "de"]);
  });

  it("maps UI locales to library locales", () => {
    expect(libraryLocale("zh_CN")).toBe("zh-CN");
    expect(libraryLocale("de-DE")).toBe("de");
    expect(libraryLocale("")).toBe("tr");
  });

  it("builds the folder tree and excludes a subtree", () => {
    const tree = buildFolderTree([
      folder("c", "a", "child"),
      folder("a", null, "b-root"),
      folder("b", null, "a-root"),
      folder("orphan", "missing"),
    ]);
    expect(flattenFolderTree(tree).map((f) => [f.uuid, f.depth])).toEqual([
      ["b", 0],
      ["a", 0],
      ["c", 1],
      ["orphan", 0],
    ]);
    expect([...folderSubtreeIds(tree, "a")].sort()).toEqual(["a", "c"]);
  });

  it("parses tags and recognises the folder-not-empty conflict", () => {
    expect(parseTags(" a, b ,a,,")).toEqual(["a", "b"]);
    expect(
      isFolderNotEmptyError(
        new ApiError({
          status: 409,
          code: "LIBRARY_FOLDER_NOT_EMPTY",
          message: "",
        }),
      ),
    ).toBe(true);
    expect(
      isFolderNotEmptyError(
        new ApiError({ status: 404, code: "NOT_FOUND", message: "" }),
      ),
    ).toBe(false);
  });
});
