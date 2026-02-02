import { useUnsavedChanges } from "./useUnsavedChanges";
import { act, render } from "@testing-library/react";
import { useRef } from "react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("useUnsavedChanges", () => {
  // Store original addEventListener/removeEventListener
  let addEventListenerSpy: ReturnType<typeof vi.spyOn>;
  let removeEventListenerSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    addEventListenerSpy = vi.spyOn(window, "addEventListener");
    removeEventListenerSpy = vi.spyOn(window, "removeEventListener");
  });

  afterEach(() => {
    addEventListenerSpy.mockRestore();
    removeEventListenerSpy.mockRestore();
  });

  // Test component that exposes hook result via callback
  const createTestComponent = (
    onResult: (result: ReturnType<typeof useUnsavedChanges>) => void,
  ) => {
    return function TestComponent({ hasChanges }: { hasChanges: boolean }) {
      const result = useUnsavedChanges(hasChanges);
      // Use a ref to avoid the callback changing on every render
      const callbackRef = useRef(onResult);
      callbackRef.current = onResult;
      callbackRef.current(result);
      return null;
    };
  };

  // Helper to create a working router wrapper that actually renders the hook
  const renderWithRouter = (hasChanges: boolean) => {
    let latestResult: ReturnType<typeof useUnsavedChanges>;
    const TestComponent = createTestComponent((result) => {
      latestResult = result;
    });

    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: <TestComponent hasChanges={hasChanges} />,
        },
        {
          path: "/other",
          element: <div>Other</div>,
        },
      ],
      { initialEntries: ["/"] },
    );

    const { rerender, unmount } = render(<RouterProvider router={router} />);

    return {
      router,
      getResult: () => latestResult!,
      rerender: (newHasChanges: boolean) => {
        const newRouter = createMemoryRouter(
          [
            {
              path: "/",
              element: <TestComponent hasChanges={newHasChanges} />,
            },
            {
              path: "/other",
              element: <div>Other</div>,
            },
          ],
          { initialEntries: ["/"] },
        );
        rerender(<RouterProvider router={newRouter} />);
      },
      unmount,
    };
  };

  describe("beforeunload event handling", () => {
    it("should add beforeunload listener when hasChanges is true", () => {
      renderWithRouter(true);

      expect(addEventListenerSpy).toHaveBeenCalledWith(
        "beforeunload",
        expect.any(Function),
      );
    });

    it("should not add beforeunload listener when hasChanges is false", () => {
      addEventListenerSpy.mockClear();
      renderWithRouter(false);

      const beforeUnloadCalls = addEventListenerSpy.mock.calls.filter(
        (call: [string, EventListenerOrEventListenerObject]) =>
          call[0] === "beforeunload",
      );
      expect(beforeUnloadCalls).toHaveLength(0);
    });

    it("should remove beforeunload listener on unmount", () => {
      const { unmount } = renderWithRouter(true);

      // Get the handler that was added
      const beforeUnloadHandler = addEventListenerSpy.mock.calls.find(
        (call: [string, EventListenerOrEventListenerObject]) =>
          call[0] === "beforeunload",
      )?.[1];

      unmount();

      expect(removeEventListenerSpy).toHaveBeenCalledWith(
        "beforeunload",
        beforeUnloadHandler,
      );
    });

    it("should call preventDefault on beforeunload event when hasChanges is true", () => {
      renderWithRouter(true);

      // Get the handler that was registered
      const beforeUnloadHandler = addEventListenerSpy.mock.calls.find(
        (call: [string, EventListenerOrEventListenerObject]) =>
          call[0] === "beforeunload",
      )?.[1] as EventListener;

      expect(beforeUnloadHandler).toBeDefined();

      // Create a mock event
      const mockEvent = {
        preventDefault: vi.fn(),
        returnValue: undefined as string | undefined,
      } as unknown as BeforeUnloadEvent;

      // Call the handler
      beforeUnloadHandler(mockEvent);

      expect(mockEvent.preventDefault).toHaveBeenCalled();
      expect(mockEvent.returnValue).toBe("");
    });
  });

  describe("blocker dialog state", () => {
    it("should initially have showBlockerDialog as false", () => {
      const { getResult } = renderWithRouter(true);

      expect(getResult().showBlockerDialog).toBe(false);
    });

    it("should return proceedNavigation and cancelNavigation functions", () => {
      const { getResult } = renderWithRouter(true);

      expect(typeof getResult().proceedNavigation).toBe("function");
      expect(typeof getResult().cancelNavigation).toBe("function");
    });

    it("should have showBlockerDialog false when hasChanges is false", () => {
      const { getResult } = renderWithRouter(false);

      expect(getResult().showBlockerDialog).toBe(false);
    });
  });

  describe("proceedNavigation and cancelNavigation", () => {
    it("proceedNavigation should be callable without error when blocker is not active", () => {
      const { getResult } = renderWithRouter(true);

      // Should not throw when called
      expect(() => {
        act(() => {
          getResult().proceedNavigation();
        });
      }).not.toThrow();
    });

    it("cancelNavigation should be callable without error when blocker is not active", () => {
      const { getResult } = renderWithRouter(true);

      // Should not throw when called
      expect(() => {
        act(() => {
          getResult().cancelNavigation();
        });
      }).not.toThrow();
    });
  });

  describe("integration with react-router blocker", () => {
    it("should show blocker dialog when trying to navigate with unsaved changes", () => {
      let latestResult: ReturnType<typeof useUnsavedChanges>;
      const TestComponent = createTestComponent((result) => {
        latestResult = result;
      });

      const router = createMemoryRouter(
        [
          {
            path: "/",
            element: <TestComponent hasChanges={true} />,
          },
          {
            path: "/other",
            element: <div>Other</div>,
          },
        ],
        { initialEntries: ["/"] },
      );

      render(<RouterProvider router={router} />);

      // Try to navigate
      act(() => {
        router.navigate("/other");
      });

      // The blocker should be active now
      expect(latestResult!.showBlockerDialog).toBe(true);
    });

    it("should not show blocker when hasChanges is false", () => {
      let latestResult: ReturnType<typeof useUnsavedChanges>;
      const TestComponent = createTestComponent((result) => {
        latestResult = result;
      });

      const router = createMemoryRouter(
        [
          {
            path: "/",
            element: <TestComponent hasChanges={false} />,
          },
          {
            path: "/other",
            element: <div>Other</div>,
          },
        ],
        { initialEntries: ["/"] },
      );

      render(<RouterProvider router={router} />);

      // Try to navigate
      act(() => {
        router.navigate("/other");
      });

      // Navigation should proceed without blocking
      expect(latestResult!.showBlockerDialog).toBe(false);
      expect(router.state.location.pathname).toBe("/other");
    });

    it("should call proceedNavigation correctly when blocker is active", () => {
      let latestResult: ReturnType<typeof useUnsavedChanges>;
      const TestComponent = createTestComponent((result) => {
        latestResult = result;
      });

      const router = createMemoryRouter(
        [
          {
            path: "/",
            element: <TestComponent hasChanges={true} />,
          },
          {
            path: "/other",
            element: <div>Other</div>,
          },
        ],
        { initialEntries: ["/"] },
      );

      render(<RouterProvider router={router} />);

      // Try to navigate (this should be blocked)
      act(() => {
        router.navigate("/other");
      });

      // Blocker should be active
      expect(latestResult!.showBlockerDialog).toBe(true);

      // Proceed with navigation
      act(() => {
        latestResult!.proceedNavigation();
      });

      // Should have navigated
      expect(router.state.location.pathname).toBe("/other");
    });

    it("should call cancelNavigation correctly when blocker is active", () => {
      let latestResult: ReturnType<typeof useUnsavedChanges>;
      const TestComponent = createTestComponent((result) => {
        latestResult = result;
      });

      const router = createMemoryRouter(
        [
          {
            path: "/",
            element: <TestComponent hasChanges={true} />,
          },
          {
            path: "/other",
            element: <div>Other</div>,
          },
        ],
        { initialEntries: ["/"] },
      );

      render(<RouterProvider router={router} />);

      // Try to navigate (this should be blocked)
      act(() => {
        router.navigate("/other");
      });

      // Blocker should be active
      expect(latestResult!.showBlockerDialog).toBe(true);

      // Cancel navigation
      act(() => {
        latestResult!.cancelNavigation();
      });

      // Should stay on current page
      expect(router.state.location.pathname).toBe("/");
      expect(latestResult!.showBlockerDialog).toBe(false);
    });
  });
});
