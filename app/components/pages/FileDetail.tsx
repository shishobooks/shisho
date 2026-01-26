import { ArrowLeft, BookOpen, Pencil } from "lucide-react";
import { useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import FileChaptersTab, {
  type ChaptersActionState,
  type FileChaptersTabHandle,
} from "@/components/files/FileChaptersTab";
import FileDetailsTab from "@/components/files/FileDetailsTab";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useBook } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";
import { FileTypeCBZ, type File } from "@/types";
import { getFilename } from "@/utils/format";

const validTabs = ["details", "chapters"] as const;
type TabValue = (typeof validTabs)[number];

const FileDetail = () => {
  const { fileId, bookId, libraryId, tab } = useParams<{
    fileId: string;
    bookId: string;
    libraryId: string;
    tab?: string;
  }>();
  const navigate = useNavigate();

  // Derive active tab from URL param, defaulting to "details"
  const activeTab: TabValue = validTabs.includes(tab as TabValue)
    ? (tab as TabValue)
    : "details";

  // Navigate to new tab URL
  const handleTabChange = (value: string) => {
    const basePath = `/libraries/${libraryId}/books/${bookId}/files/${fileId}`;
    if (value === "details") {
      navigate(basePath);
    } else {
      navigate(`${basePath}/${value}`);
    }
  };

  // Edit states for each tab
  const [editingFile, setEditingFile] = useState<File | null>(null);
  const [isEditingChapters, setIsEditingChapters] = useState(false);

  // Chapters tab ref and action state
  const chaptersRef = useRef<FileChaptersTabHandle>(null);
  const [chaptersActionState, setChaptersActionState] =
    useState<ChaptersActionState>({ isSaving: false, canSave: false });

  const bookQuery = useBook(bookId);
  const libraryQuery = useLibrary(libraryId);

  // Find file in book.files array
  const file = bookQuery.data?.files?.find(
    (f) => f.id === parseInt(fileId || "0"),
  );

  if (bookQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!bookQuery.isSuccess || !bookQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Book Not Found</h1>
          <p className="text-muted-foreground mb-6">
            The book you're looking for doesn't exist or may have been removed.
          </p>
          <Button asChild>
            <Link to={`/libraries/${libraryId}`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Home
            </Link>
          </Button>
        </div>
      </LibraryLayout>
    );
  }

  if (!file) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">File Not Found</h1>
          <p className="text-muted-foreground mb-6">
            The file you're looking for doesn't exist or may have been removed.
          </p>
          <Button asChild>
            <Link to={`/libraries/${libraryId}/books/${bookId}`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Book
            </Link>
          </Button>
        </div>
      </LibraryLayout>
    );
  }

  const book = bookQuery.data;
  const library = libraryQuery.data;
  const filename = file.name || getFilename(file.filepath);

  return (
    <LibraryLayout>
      {/* Header with breadcrumbs and back button */}
      <div className="mb-6">
        <Button asChild variant="ghost">
          <Link to={`/libraries/${libraryId}/books/${bookId}`}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Book
          </Link>
        </Button>
      </div>

      {/* Breadcrumbs */}
      <nav className="mb-4 text-xs sm:text-sm text-muted-foreground overflow-hidden">
        <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
          <li className="shrink-0">
            <Link
              className="hover:text-foreground hover:underline"
              to={`/libraries/${libraryId}`}
            >
              {library?.name || "Library"}
            </Link>
          </li>
          <li aria-hidden="true" className="shrink-0">
            ›
          </li>
          <li className="truncate max-w-[120px] sm:max-w-none">
            <Link
              className="hover:text-foreground hover:underline"
              to={`/libraries/${libraryId}/books/${bookId}`}
            >
              {book.title}
            </Link>
          </li>
          <li aria-hidden="true" className="shrink-0">
            ›
          </li>
          <li className="text-foreground truncate">{filename}</li>
        </ol>
      </nav>

      {/* File title with Edit/Save/Cancel buttons */}
      <div className="flex flex-col gap-3 mb-6">
        <h1 className="text-2xl md:text-3xl font-semibold">{filename}</h1>
        <div className="flex items-center gap-2">
          {/* Read button for CBZ files */}
          {file.file_type === FileTypeCBZ && (
            <Button asChild size="sm">
              <Link
                to={`/libraries/${libraryId}/books/${bookId}/files/${fileId}/read`}
              >
                <BookOpen className="h-4 w-4 sm:mr-2" />
                <span className="hidden sm:inline">Read</span>
              </Link>
            </Button>
          )}

          {activeTab === "chapters" && isEditingChapters ? (
            <>
              <Button
                disabled={
                  !chaptersActionState.canSave || chaptersActionState.isSaving
                }
                onClick={() => chaptersRef.current?.save()}
                size="sm"
              >
                {chaptersActionState.isSaving ? "Saving..." : "Save"}
              </Button>
              <Button
                disabled={chaptersActionState.isSaving}
                onClick={() => chaptersRef.current?.cancel()}
                size="sm"
                variant="outline"
              >
                Cancel
              </Button>
            </>
          ) : (
            <Button
              onClick={() => {
                if (activeTab === "details") {
                  setEditingFile(file);
                } else {
                  setIsEditingChapters(true);
                }
              }}
              size="sm"
              variant="outline"
            >
              <Pencil className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Edit</span>
            </Button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <Tabs onValueChange={handleTabChange} value={activeTab}>
        <TabsList>
          <TabsTrigger value="details">Details</TabsTrigger>
          <TabsTrigger value="chapters">Chapters</TabsTrigger>
        </TabsList>

        <TabsContent value="details">
          <FileDetailsTab file={file} />
        </TabsContent>

        <TabsContent value="chapters">
          <FileChaptersTab
            file={file}
            isEditing={isEditingChapters}
            onActionStateChange={setChaptersActionState}
            onEditingChange={setIsEditingChapters}
            ref={chaptersRef}
          />
        </TabsContent>
      </Tabs>

      {/* FileEditDialog for editing file details */}
      {editingFile && (
        <FileEditDialog
          file={editingFile}
          onOpenChange={(open) => {
            if (!open) {
              setEditingFile(null);
            }
          }}
          open={!!editingFile}
        />
      )}
    </LibraryLayout>
  );
};

export default FileDetail;
