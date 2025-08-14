import { uniqBy } from "lodash";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { useBooks } from "@/hooks/queries/books";

const Home = () => {
  const booksQuery = useBooks();

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        {booksQuery.isLoading ? (
          <LoadingSpinner />
        ) : !booksQuery.isSuccess ? (
          <div>error</div>
        ) : (
          <div className="flex flex-wrap gap-4 p-4">
            {booksQuery.data.books.map((book) => (
              <div
                className="w-32"
                key={book.id}
                title={`${book.title}${book.authors ? `\nBy ${book.authors.map((a) => a.name).join(", ")}` : ""}`}
              >
                <Link className="group cursor-pointer" to={`/books/${book.id}`}>
                  <img
                    alt={`${book.title} Cover`}
                    className="h-48 object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = "none";
                      (
                        e.target as HTMLImageElement
                      ).nextElementSibling!.textContent = "no cover";
                    }}
                    src={`/api/books/${book.id}/cover`}
                  />
                  <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
                    {book.title}
                  </div>
                </Link>
                {book.authors && (
                  <div className="mt-1 text-sm line-clamp-2 text-neutral-500 dark:text-neutral-500">
                    {book.authors.map((a) => a.name).join(", ")}
                  </div>
                )}
                {book.files && (
                  <div className="mt-2 flex gap-2 text-sm">
                    {uniqBy(book.files, "file_type").map((f) => (
                      <Badge className="uppercase" key={f.id} variant="subtle">
                        {f.file_type}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default Home;
