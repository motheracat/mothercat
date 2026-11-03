"use client";

import Image from "next/image";
import { useEffect, useRef, useState, type ChangeEvent } from "react";

import {
  formatOrganizationError,
  listStaff,
  OrganizationApiError,
  uploadStaffProfilePicture,
  type StaffMember,
} from "@/lib/organization/api";

const maxProfilePictureBytes = 5 * 1024 * 1024;
const acceptedImageTypes = new Set(["image/jpeg", "image/png", "image/gif"]);

type ProfilePictureUploadProps = {
  staffId?: string;
  staffNumber?: string;
  name: string;
  profileImageUrl: string | null;
  avatarWrapClassName: string;
  avatarClassName: string;
  initialsClassName?: string;
  statusClassName?: string;
  statusLabel?: string;
  canUpload?: boolean;
  onStaffResolved?: (staff: StaffMember) => void;
  onStaffLookupError?: (message: string, reason: "missing" | "request") => void;
  onStaffUpdated?: (staff: StaffMember) => void;
};

function uploadErrorMessage(error: unknown): string {
  const formatted = formatOrganizationError(error, "Unable to upload the profile picture. Try again.");
  if (!(error instanceof OrganizationApiError)) return formatted;

  if (error.code === "invalid_image" || error.code === "unsupported_image_type") {
    return "Choose a valid JPEG, PNG, or GIF image.";
  }
  if (error.status === 413 || error.code === "image_too_large" || error.code === "request_too_large") {
    return "The image must be no larger than 5 MiB.";
  }
  if (error.status === 403 || error.code === "permission_denied") {
    return "You do not have permission to change this profile picture.";
  }
  if (error.status === 404 || error.code === "staff_not_found") {
    return "This staff record could not be found. Refresh the profile and try again.";
  }
  return formatted;
}

export function ProfilePictureUpload({
  staffId,
  staffNumber,
  name,
  profileImageUrl,
  avatarWrapClassName,
  avatarClassName,
  initialsClassName,
  statusClassName,
  statusLabel,
  canUpload = true,
  onStaffResolved,
  onStaffLookupError,
  onStaffUpdated,
}: ProfilePictureUploadProps) {
  const [resolvedStaffId, setResolvedStaffId] = useState(staffId ?? null);
  const [lookupState, setLookupState] = useState<"loading" | "ready" | "failed">(
    staffId ? "ready" : staffNumber ? "loading" : "failed",
  );
  const [lookupError, setLookupError] = useState<string | null>(
    staffId || staffNumber ? null : "Your staff number is unavailable. Refresh your profile or contact an administrator.",
  );
  const [imageUrl, setImageUrl] = useState(profileImageUrl);
  const [imageFailed, setImageFailed] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState<string | null>(null);
  const uploadingRef = useRef(false);

  useEffect(() => {
    if (staffId || !staffNumber) return;

    let active = true;
    void listStaff({ limit: 100, offset: 0, search: staffNumber })
      .then(({ staff }) => {
        if (!active) return;
        const currentStaff = staff.find((member) => member.staff_id === staffNumber);
        if (!currentStaff) {
          const message = "This account has no organization staff profile matching its staff number. Profile edits and picture uploads require a staff record; the identity account alone is not enough.";
          setLookupState("failed");
          setLookupError(message);
          onStaffLookupError?.(message, "missing");
          return;
        }
        setResolvedStaffId(currentStaff.id);
        setImageUrl(currentStaff.profile_image_url);
        setImageFailed(false);
        setLookupState("ready");
        onStaffResolved?.(currentStaff);
      })
      .catch((error: unknown) => {
        if (!active) return;
        const message = formatOrganizationError(error, "Unable to find your staff record. Refresh your profile and try again.");
        setLookupError(message);
        setLookupState("failed");
        onStaffLookupError?.(message, "request");
      });

    return () => { active = false; };
  }, [staffId, staffNumber, onStaffLookupError, onStaffResolved]);

  async function handleFileChange(event: ChangeEvent<HTMLInputElement>) {
    const input = event.currentTarget;
    const file = input.files?.[0];
    input.value = "";
    if (!file || uploadingRef.current) return;

    setUploadError(null);
    setAnnouncement(null);
    if (file.size === 0) {
      setUploadError("Choose a non-empty JPEG, PNG, or GIF image.");
      return;
    }
    if (!acceptedImageTypes.has(file.type)) {
      setUploadError("Choose a JPEG, PNG, or GIF image.");
      return;
    }
    if (file.size > maxProfilePictureBytes) {
      setUploadError("The image must be no larger than 5 MiB.");
      return;
    }
    if (!resolvedStaffId || lookupState !== "ready") {
      setUploadError(lookupError ?? "Your staff record is not available. Refresh your profile and try again.");
      return;
    }

    uploadingRef.current = true;
    setUploading(true);
    try {
      const updatedStaff = await uploadStaffProfilePicture(resolvedStaffId, file);
      setImageUrl(updatedStaff.profile_image_url);
      setImageFailed(false);
      onStaffUpdated?.(updatedStaff);
      setAnnouncement("Profile picture updated.");
    } catch (error) {
      setUploadError(uploadErrorMessage(error));
    } finally {
      uploadingRef.current = false;
      setUploading(false);
    }
  }

  const canShowImage = imageUrl && (imageUrl.startsWith("/") || /^https?:\/\//i.test(imageUrl));

  return (
    <div className="profile-picture-uploader">
      <div className={avatarWrapClassName}>
        {canShowImage && !imageFailed ? (
          <Image
            src={imageUrl}
            alt=""
            fill
            sizes="76px"
            unoptimized
            className="profile-picture-image"
            onError={() => setImageFailed(true)}
          />
        ) : (
          <span className={initialsClassName ?? avatarClassName} aria-hidden="true">
            {name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("") || "U"}
          </span>
        )}
        {statusClassName ? (
          <span
            className={statusClassName}
            {...(statusLabel ? { role: "img", "aria-label": statusLabel } : { "aria-hidden": true })}
          />
        ) : null}
      </div>
      {canUpload ? (
        <label
          className={`button-secondary profile-picture-upload-control${lookupState !== "ready" ? " is-disabled" : ""}`}
          aria-disabled={uploading || lookupState !== "ready"}
        >
          {lookupState === "failed"
            ? "Profile picture unavailable"
            : uploading
              ? "Uploading…"
              : imageUrl
                ? "Change profile picture"
                : "Upload profile picture"}
          <input
            className="sr-only"
            type="file"
            accept="image/jpeg,image/png,image/gif"
            disabled={uploading || lookupState !== "ready"}
            onChange={(event) => void handleFileChange(event)}
          />
        </label>
      ) : null}
      {canUpload && lookupState === "loading" ? <p className="profile-picture-feedback" role="status">Finding your staff record…</p> : null}
      {lookupError && !onStaffLookupError ? <p className="profile-picture-feedback" role="alert">{lookupError}</p> : null}
      {uploading ? <p className="profile-picture-feedback" role="status">Uploading profile picture…</p> : null}
      {uploadError ? <p className="profile-picture-feedback" role="alert">{uploadError}</p> : null}
      {announcement ? <p className="profile-picture-feedback" role="status">{announcement}</p> : null}
    </div>
  );
}
