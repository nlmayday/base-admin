import { beforeEach, describe, expect, it, vi } from "vitest";
import { getUploadPolicy, uploadAttachment } from "@/api/file";
import { message } from "@/utils/message";
import { ApiError } from "@/utils/http/errors";
import { useAttachmentUpload } from "./useAttachmentUpload";

vi.mock("@/api/file", () => ({
  getUploadPolicy: vi.fn(),
  uploadAttachment: vi.fn()
}));
vi.mock("@/utils/message", () => ({ message: vi.fn() }));

const flush = () => new Promise(resolve => setTimeout(resolve));
const file = (name: string, size: number, type: string) =>
  new File([new Uint8Array(size)], name, { type });

beforeEach(() => {
  vi.mocked(getUploadPolicy).mockResolvedValue({
    extensions: ["png", "pdf"],
    maxBytes: 1024
  });
  vi.mocked(uploadAttachment).mockReset();
  vi.mocked(message).mockReset();
});

describe("useAttachmentUpload", () => {
  it("checks the upload rules before sending", async () => {
    const up = useAttachmentUpload(false);
    await flush();
    expect(
      await up.upload(file("a.exe", 10, "application/octet-stream"))
    ).toBeNull();
    expect(vi.mocked(message).mock.calls[0][0]).toContain("不支持 .exe");
    expect(
      await up.upload(file("big.pdf", 2048, "application/pdf"))
    ).toBeNull();
    expect(vi.mocked(message).mock.calls[1][0]).toContain("不能超过 1.0 KB");
    expect(uploadAttachment).not.toHaveBeenCalled();
  });

  it("accepts only images in image mode", async () => {
    const up = useAttachmentUpload(true);
    await flush();
    expect(await up.upload(file("a.pdf", 10, "application/pdf"))).toBeNull();
    expect(vi.mocked(message).mock.calls[0][0]).toBe("只能上传图片");
  });

  it("returns the attachment, or reports the server's message", async () => {
    const done = {
      id: "1",
      name: "a.png",
      url: "/api/v1/files/a/x.png",
      size: 10,
      kind: "image" as const,
      mimeType: "image/png"
    };
    vi.mocked(uploadAttachment).mockResolvedValueOnce(done);
    const up = useAttachmentUpload(true);
    await flush();
    expect(await up.upload(file("a.png", 10, "image/png"))).toEqual(done);
    expect(up.uploading.value).toBe(false);

    vi.mocked(uploadAttachment).mockRejectedValueOnce(
      new ApiError({ code: 60002, message: "文件内容与扩展名不符" })
    );
    expect(await up.upload(file("b.png", 10, "image/png"))).toBeNull();
    expect(message).toHaveBeenLastCalledWith("文件内容与扩展名不符", {
      type: "error"
    });
  });

  it("formats sizes", () => {
    const { sizeText } = useAttachmentUpload(false);
    expect(sizeText(193)).toBe("193 B");
    expect(sizeText(1536)).toBe("1.5 KB");
    expect(sizeText(20 * 1024 * 1024)).toBe("20 MB");
  });
});
