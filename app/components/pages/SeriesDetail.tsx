import { useEffect } from "react";
import { useNavigate, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { useSeries } from "@/hooks/queries/series";

const SeriesDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const seriesId = id ? parseInt(id, 10) : undefined;

  const seriesQuery = useSeries(seriesId);

  useEffect(() => {
    if (seriesId && seriesQuery.isSuccess) {
      // Redirect to home page with series_id query param
      navigate(`/libraries/${libraryId}?series_id=${seriesId}`, {
        replace: true,
      });
    }
  }, [seriesId, seriesQuery.isSuccess, navigate, libraryId]);

  if (seriesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!seriesQuery.isSuccess || !seriesQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Series Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The series you're looking for doesn't exist or may have been
              removed.
            </p>
          </div>
        </div>
      </div>
    );
  }

  // Show loading while redirecting
  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <LoadingSpinner />
      </div>
    </div>
  );
};

export default SeriesDetail;
