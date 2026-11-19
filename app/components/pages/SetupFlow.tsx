import { CircleAlert, CircleCheck, CircleDashed, Loader2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import { LibraryStep } from "@/components/setup/LibraryStep";
import {
  pluginChoices,
  pluginKey,
  recommendPlugins,
} from "@/components/setup/pluginRecommendations";
import { PluginsStep } from "@/components/setup/PluginsStep";
import {
  clearSetupDraft,
  libraryStepErrors,
  loadSetupDraft,
  saveSetupDraft,
  type LibraryStepErrors,
  type SetupDraft,
} from "@/components/setup/setupDraft";
import { SetupLayout } from "@/components/setup/SetupLayout";
import { Button } from "@/components/ui/button";
import { useGettingStartedPanel } from "@/contexts/GettingStarted";
import { useConfig } from "@/hooks/queries/config";
import { useDetectFileTypes } from "@/hooks/queries/filesystem";
import { useCreateLibrary } from "@/hooks/queries/libraries";
import {
  useInstallPlugin,
  usePluginsAvailable,
  usePluginsInstalled,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";
import { useAuth } from "@/hooks/useAuth";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import { requestErrorMessage, toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import { CoverAspectRatioBook, PluginStatusActive } from "@/types";

const STEPS = ["library", "plugins"] as const;
type UrlStep = (typeof STEPS)[number];

type TaskStatus = "waiting" | "running" | "done" | "failed";

interface Task {
  key: string;
  label: string;
  status: TaskStatus;
}

const TaskIcon = ({ status }: { status: TaskStatus }) => {
  const className = "h-5 w-5 shrink-0";
  switch (status) {
    case "running":
      return (
        <Loader2
          aria-hidden="true"
          className={cn(className, "animate-spin text-primary")}
        />
      );
    case "done":
      return (
        <CircleCheck
          aria-hidden="true"
          className={cn(className, "text-primary")}
        />
      );
    case "failed":
      return (
        <CircleAlert
          aria-hidden="true"
          className={cn(className, "text-destructive")}
        />
      );
    default:
      return (
        <CircleDashed
          aria-hidden="true"
          className={cn(className, "text-muted-foreground")}
        />
      );
  }
};

const STATUS_TEXT: Record<TaskStatus, string> = {
  waiting: "waiting",
  running: "in progress",
  done: "done",
  failed: "failed",
};

/**
 * Setup after the admin account: the Library step, then the enrichment step,
 * then Finish, which installs the chosen plugins and only then creates the
 * library, so its first scan runs with them. The step is in the URL and the
 * choices are in browser storage, so a reload resumes where the admin was.
 */
const SetupFlow = () => {
  usePageTitle("Setup");

  const { step } = useParams();
  const navigate = useNavigate();
  const { user, can } = useAuth();
  const { requestPulse } = useGettingStartedPanel();
  const canInstall = can(["config:read", "config:write"]);

  const [draft, setDraft] = useState<SetupDraft>(() =>
    loadSetupDraft(user?.id ?? 0),
  );
  // The draft is saved only once the admin changes something, and not after
  // Setup ends, so visiting Setup leaves nothing behind.
  const persist = useRef(false);
  useEffect(() => {
    if (persist.current) saveSetupDraft(draft);
  }, [draft]);
  const update = useCallback((changes: Partial<SetupDraft>) => {
    persist.current = true;
    setDraft((prev) => ({ ...prev, ...changes }));
  }, []);
  const forgetDraft = () => {
    persist.current = false;
    clearSetupDraft();
  };

  const [showErrors, setShowErrors] = useState(false);
  const errors: LibraryStepErrors = libraryStepErrors(draft);
  const libraryReady = !errors.name && !errors.paths;

  // Detection starts as soon as there are folders, so its answer is usually
  // ready by the time the enrichment step opens.
  const paths = useMemo(
    () => draft.paths.map((p) => p.trim()).filter((p) => p !== ""),
    [draft.paths],
  );
  // Typed folders settle first, so a pause partway through a path doesn't
  // walk its parent. The enrichment step needs the folders as they are.
  const detectPaths = useDebounce(paths, 800, {
    immediate: () => step === "plugins",
  });
  const detection = useDetectFileTypes(detectPaths);
  const detectionCurrent = detectPaths.join("\n") === paths.join("\n");

  // The repository is fetched in the background too.
  const availableQuery = usePluginsAvailable();
  const installedQuery = usePluginsInstalled();
  const installedKeys = useMemo(
    () => new Set((installedQuery.data ?? []).map(pluginKey)),
    [installedQuery.data],
  );
  const officialAvailable = (availableQuery.data ?? []).some(
    (p) => p.is_official,
  );
  const unreachable =
    (availableQuery.isError && !availableQuery.data) ||
    (availableQuery.isSuccess && !officialAvailable);
  const choices = useMemo(
    () =>
      pluginChoices(
        availableQuery.data ?? [],
        recommendPlugins(detection.data),
      ),
    [availableQuery.data, detection.data],
  );
  const selected = useMemo(
    () =>
      new Set(
        (draft.plugins ?? choices.preselectedKeys).filter(
          (key) => !installedKeys.has(key),
        ),
      ),
    [draft.plugins, choices.preselectedKeys, installedKeys],
  );

  const { data: config } = useConfig();
  const devLibraryPath = import.meta.env.DEV
    ? config?.dev_library_path || undefined
    : undefined;

  const installPlugin = useInstallPlugin();
  const createLibrary = useCreateLibrary();
  const [tasks, setTasks] = useState<Task[] | null>(null);
  const [createError, setCreateError] = useState<string | null>(null);

  const setTaskStatus = (key: string, status: TaskStatus) =>
    setTasks((prev) =>
      prev ? prev.map((t) => (t.key === key ? { ...t, status } : t)) : prev,
    );

  const createTheLibrary = async () => {
    setCreateError(null);
    setTaskStatus("library", "running");
    try {
      const library = await createLibrary.mutateAsync({
        payload: {
          name: draft.name.trim(),
          library_paths: paths,
          organize_file_structure: draft.organize,
          cover_aspect_ratio: CoverAspectRatioBook,
        },
      });
      setTaskStatus("library", "done");
      forgetDraft();
      // The library page shows the first scan; the pill pulses once to point
      // at what's next, without the panel covering the arriving books.
      requestPulse();
      navigate(`/libraries/${library.id}`, { replace: true });
    } catch (error) {
      setTaskStatus("library", "failed");
      setCreateError(requestErrorMessage(error, "Failed to create library"));
    }
  };

  // Installs each chosen plugin, then creates the library. A failed install
  // is reported and the rest carries on: the "Add metadata plugins" step in
  // Getting Started stays open for it.
  const finish = async (plugins: AvailablePlugin[]) => {
    const toInstall = plugins.flatMap((plugin) => {
      const version = plugin.versions.find((v) => v.compatible);
      return version ? [{ plugin, version }] : [];
    });
    setTasks([
      ...toInstall.map(({ plugin }) => ({
        key: pluginKey(plugin),
        label: `Install ${plugin.name}`,
        status: "waiting" as const,
      })),
      { key: "library", label: "Create your library", status: "waiting" },
    ]);
    for (const { plugin, version } of toInstall) {
      const key = pluginKey(plugin);
      setTaskStatus(key, "running");
      try {
        const installed = await installPlugin.mutateAsync({
          scope: plugin.scope,
          id: plugin.id,
          name: plugin.name,
          version: version.version,
        });
        setTaskStatus(key, "done");
        if (installed.status !== PluginStatusActive) {
          toast.warning(`${plugin.name} was installed but couldn't start`, {
            description: "Check it under Administration > Plugins.",
          });
        }
      } catch (error) {
        setTaskStatus(key, "failed");
        toastRequestError(error, `Failed to install ${plugin.name}`, {
          description: `${plugin.name} wasn't installed. You can add it later from Administration > Plugins.`,
        });
      }
    }
    await createTheLibrary();
  };

  const selectedPlugins = () =>
    [...choices.recommended, ...choices.others].filter((p) =>
      selected.has(pluginKey(p)),
    );

  const skipSetup = () => {
    forgetDraft();
    navigate("/");
  };

  const handleLibraryContinue = () => {
    if (!libraryReady) {
      setShowErrors(true);
      return;
    }
    if (canInstall) {
      navigate("/setup/plugins");
    } else {
      void finish([]);
    }
  };

  const toggle = (key: string, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(key);
    else next.delete(key);
    update({ plugins: [...next] });
  };

  if (tasks) {
    return (
      <SetupLayout
        description="Hold on while Shisho gets your library ready."
        step="finish"
        title="Finishing setup"
      >
        <ul aria-label="Setup progress" className="mx-auto max-w-sm space-y-3">
          {tasks.map((task) => (
            <li className="flex items-center gap-3 text-sm" key={task.key}>
              <TaskIcon status={task.status} />
              <span
                className={cn(
                  task.status === "waiting" && "text-muted-foreground",
                )}
              >
                {task.label}
                <span className="sr-only">, {STATUS_TEXT[task.status]}</span>
              </span>
            </li>
          ))}
        </ul>
        {createError && (
          <div className="mx-auto mt-6 max-w-sm space-y-3">
            <p className="text-sm text-destructive" role="alert">
              {createError}
            </p>
            <div className="flex gap-2">
              <Button
                onClick={() => {
                  setTasks(null);
                  setCreateError(null);
                  navigate("/setup/library");
                }}
                variant="outline"
              >
                Back
              </Button>
              <Button
                disabled={createLibrary.isPending}
                onClick={() => void createTheLibrary()}
              >
                Try again
              </Button>
            </div>
          </div>
        )}
      </SetupLayout>
    );
  }

  if (!STEPS.includes(step as UrlStep)) {
    return <Navigate replace to="/setup/library" />;
  }

  if (step === "plugins") {
    // The enrichment step needs a library to make and a role that installs
    // plugins.
    if (!libraryReady || !canInstall) {
      return <Navigate replace to="/setup/library" />;
    }
    return (
      <SetupLayout
        description="Plugins fill in covers, descriptions, series, and more. Ticking a plugin installs it and approves what it can access."
        onSkip={skipSetup}
        step="plugins"
        title="Add metadata plugins"
      >
        <PluginsStep
          choices={choices}
          // Detection only shapes the recommendations, so when it fails the
          // step offers the main plugins, as it does for empty folders.
          detected={detection.isError ? undefined : detection.data}
          installed={installedKeys}
          loading={
            !detectionCurrent || detection.isLoading
              ? "Looking through your folders…"
              : availableQuery.isLoading || installedQuery.isLoading
                ? "Loading plugins…"
                : null
          }
          onBack={() => navigate("/setup/library")}
          onFinish={() => void finish(selectedPlugins())}
          onSkipStep={() => void finish([])}
          onToggle={toggle}
          selected={selected}
          unreachable={unreachable}
        />
      </SetupLayout>
    );
  }

  return (
    <SetupLayout
      description="Point Shisho at the folders that hold your books. It creates your library when you finish."
      onSkip={skipSetup}
      step="library"
      title="Where are your books?"
    >
      <LibraryStep
        continueLabel={canInstall ? "Continue" : "Finish setup"}
        devLibraryPath={devLibraryPath}
        draft={draft}
        errors={showErrors ? errors : {}}
        onChange={update}
        onContinue={handleLibraryContinue}
      />
    </SetupLayout>
  );
};

export default SetupFlow;
