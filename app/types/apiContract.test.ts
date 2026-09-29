import { describe, expectTypeOf, it } from "vitest";

import type * as APIKeys from "@/types/generated/apikeys";
import type * as Books from "@/types/generated/books";
import type * as Cache from "@/types/generated/cache";
import type * as Chapters from "@/types/generated/chapters";
import type * as Libraries from "@/types/generated/libraries";
import type * as Lists from "@/types/generated/lists";

// These assertions run under `tsc` (tsconfig.test.json), so they check what
// tygo generates from the Go types. A Go shape change that breaks one of
// them fails `pnpm lint:types`.
describe("generated API contract", () => {
  it("serializes API keys with snake_case keys", () => {
    expectTypeOf<APIKeys.APIKey>().toHaveProperty("user_id");
    expectTypeOf<APIKeys.APIKey>().toHaveProperty("created_at");
    expectTypeOf<APIKeys.APIKey>().toHaveProperty("updated_at");
    expectTypeOf<APIKeys.APIKey>().toHaveProperty("last_accessed_at");
    expectTypeOf<
      Extract<
        keyof APIKeys.APIKey,
        "userId" | "createdAt" | "updatedAt" | "lastAccessedAt"
      >
    >().toBeNever();

    expectTypeOf<APIKeys.APIKeyPermission>().toHaveProperty("api_key_id");
    expectTypeOf<APIKeys.APIKeyPermission>().toHaveProperty("created_at");
    expectTypeOf<
      Extract<keyof APIKeys.APIKeyPermission, "apiKeyId" | "createdAt">
    >().toBeNever();

    expectTypeOf<APIKeys.APIKeyShortURL>().toHaveProperty("api_key_id");
    expectTypeOf<APIKeys.APIKeyShortURL>().toHaveProperty("short_code");
    expectTypeOf<APIKeys.APIKeyShortURL>().toHaveProperty("expires_at");
    expectTypeOf<
      Extract<
        keyof APIKeys.APIKeyShortURL,
        "apiKeyId" | "shortCode" | "expiresAt" | "createdAt"
      >
    >().toBeNever();
  });

  it("emits the request payloads that used to live outside types.go", () => {
    expectTypeOf<APIKeys.CreateAPIKeyPayload>().toEqualTypeOf<{
      name: string;
    }>();
    expectTypeOf<APIKeys.UpdateAPIKeyNamePayload>().toEqualTypeOf<{
      name: string;
    }>();
    expectTypeOf<Books.UpdateFileCoverPagePayload>().toEqualTypeOf<{
      page: number;
    }>();
    expectTypeOf<Cache.ClearResponse>().toHaveProperty("cleared_bytes");
    expectTypeOf<Cache.Info>().toHaveProperty("size_bytes");
  });

  it("drops the passthrough and wrapper types", () => {
    // @ts-expect-error LibraryResponse was a passthrough wrapper; the library routes return the bare Library.
    expectTypeOf<Libraries.LibraryResponse>().toBeObject();
    // @ts-expect-error The chapters list and replace routes return a bare Chapter array.
    expectTypeOf<Chapters.ChaptersResponse>().toBeObject();
    // @ts-expect-error The cache list returns a bare Info array.
    expectTypeOf<Cache.ListResponse>().toBeObject();
    // @ts-expect-error The list retrieve route returns ListResponse, like the list route.
    expectTypeOf<Lists.RetrieveListResponse>().toBeObject();
    // @ts-expect-error Cache Provider and Handler are Go internals, not API types.
    expectTypeOf<Cache.Handler>().toBeObject();
  });

  it("keeps the library summary projection", () => {
    expectTypeOf<keyof Libraries.LibrarySummary>().toEqualTypeOf<
      | "id"
      | "name"
      | "cover_aspect_ratio"
      | "download_format_preference"
      | "organize_file_structure"
    >();
  });
});
