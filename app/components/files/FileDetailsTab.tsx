import { Link, useParams } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import {
  FileRoleMain,
  FileRoleSupplement,
  FileTypeCBZ,
  FileTypeM4B,
  type File,
} from "@/types";
import {
  formatDate,
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  getFilename,
} from "@/utils/format";

function formatFileRole(role: string): string {
  switch (role) {
    case FileRoleMain:
      return "Main";
    case FileRoleSupplement:
      return "Supplement";
    default:
      return role;
  }
}

interface FileDetailsTabProps {
  file: File;
}

const FileDetailsTab = ({ file }: FileDetailsTabProps) => {
  const { libraryId } = useParams<{ libraryId: string }>();

  return (
    <div className="py-4 space-y-6">
      {/* Basic file information */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
        {/* File type */}
        <div>
          <p className="font-semibold">File Type</p>
          <div className="mt-1">
            <Badge className="uppercase" variant="secondary">
              {file.file_type}
            </Badge>
          </div>
        </div>

        {/* File role */}
        <div>
          <p className="font-semibold">File Role</p>
          <p className="text-muted-foreground">
            {formatFileRole(file.file_role)}
          </p>
        </div>

        {/* File size */}
        <div>
          <p className="font-semibold">File Size</p>
          <p className="text-muted-foreground">
            {formatFileSize(file.filesize_bytes)}
          </p>
        </div>

        {/* File name */}
        <div>
          <p className="font-semibold">File Name</p>
          <p className="text-muted-foreground break-all">
            {getFilename(file.filepath)}
          </p>
        </div>

        {/* Page count - CBZ only */}
        {file.file_type === FileTypeCBZ && file.page_count != null && (
          <div>
            <p className="font-semibold">Page Count</p>
            <p className="text-muted-foreground">{file.page_count} pages</p>
          </div>
        )}

        {/* Duration - M4B only */}
        {file.file_type === FileTypeM4B &&
          file.audiobook_duration_seconds != null && (
            <div>
              <p className="font-semibold">Duration</p>
              <p className="text-muted-foreground">
                {formatDuration(file.audiobook_duration_seconds)}
              </p>
            </div>
          )}

        {/* Bitrate - M4B only */}
        {file.file_type === FileTypeM4B &&
          file.audiobook_bitrate_bps != null && (
            <div>
              <p className="font-semibold">Bitrate</p>
              <p className="text-muted-foreground">
                {Math.round(file.audiobook_bitrate_bps / 1000)} kbps
              </p>
            </div>
          )}
      </div>

      {/* Narrators - M4B only */}
      {file.file_type === FileTypeM4B &&
        file.narrators &&
        file.narrators.length > 0 && (
          <div className="text-sm">
            <p className="font-semibold mb-2">Narrators</p>
            <div className="flex flex-wrap gap-2">
              {file.narrators.map((narrator) => (
                <Link
                  key={narrator.id}
                  to={`/libraries/${libraryId}/people/${narrator.person_id}`}
                >
                  <Badge
                    className="cursor-pointer hover:bg-secondary/80"
                    variant="secondary"
                  >
                    {narrator.person?.name ?? "Unknown"}
                  </Badge>
                </Link>
              ))}
            </div>
          </div>
        )}

      {/* Publisher and Imprint */}
      {(file.publisher || file.imprint) && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
          {file.publisher && (
            <div>
              <p className="font-semibold">Publisher</p>
              <p className="text-muted-foreground">{file.publisher.name}</p>
            </div>
          )}
          {file.imprint && (
            <div>
              <p className="font-semibold">Imprint</p>
              <p className="text-muted-foreground">{file.imprint.name}</p>
            </div>
          )}
        </div>
      )}

      {/* Release date */}
      {file.release_date && (
        <div className="text-sm">
          <p className="font-semibold">Release Date</p>
          <p className="text-muted-foreground">
            {formatDate(file.release_date)}
          </p>
        </div>
      )}

      {/* Identifiers */}
      {file.identifiers && file.identifiers.length > 0 && (
        <div className="text-sm">
          <p className="font-semibold mb-2">Identifiers</p>
          <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
            {file.identifiers.map((id, idx) => (
              <div className="contents" key={idx}>
                <span className="text-muted-foreground">
                  {formatIdentifierType(id.type)}
                </span>
                <span className="font-mono select-all">{id.value}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* URL */}
      {file.url && (
        <div className="text-sm">
          <p className="font-semibold">URL</p>
          <a
            className="text-primary hover:underline break-all"
            href={file.url}
            rel="noopener noreferrer"
            target="_blank"
          >
            {file.url}
          </a>
        </div>
      )}
    </div>
  );
};

export default FileDetailsTab;
