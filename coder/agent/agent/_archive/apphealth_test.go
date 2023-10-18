package _archive_test

// func TestAppHealth_Healthy(t *testing.T) {
// 	t.Parallel()
// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()
// 	apps := []coderagentsdk.WorkspaceApp{
// 		{
// 			Slug:        "app1",
// 			Healthcheck: coderagentsdk.Healthcheck{},
// 			Health:      coderagentsdk.WorkspaceAppHealthDisabled,
// 		},
// 		{
// 			Slug: "app2",
// 			Healthcheck: coderagentsdk.Healthcheck{
// 				// URL: We don't set the URL for this test because the setup will
// 				// create a httptest server for us and set it for us.
// 				Interval:  1,
// 				Threshold: 1,
// 			},
// 			Health: coderagentsdk.WorkspaceAppHealthInitializing,
// 		},
// 	}
// 	handlers := []http.Handler{
// 		nil,
// 		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			httpapi.Write(r.Context(), w, http.StatusOK, nil)
// 		}),
// 	}
// 	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
// 	defer closeFn()
// 	apps, err := getApps(ctx)
// 	require.NoError(t, err)
// 	require.EqualValues(t, coderagentsdk.WorkspaceAppHealthDisabled, apps[0].Health)
// 	require.Eventually(t, func() bool {
// 		apps, err := getApps(ctx)
// 		if err != nil {
// 			return false
// 		}
//
// 		return apps[1].Health == coderagentsdk.WorkspaceAppHealthHealthy
// 	}, 15*time.Second, 2*time.Second)
// }
//
// func TestAppHealth_500(t *testing.T) {
// 	t.Parallel()
// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()
// 	apps := []coderagentsdk.WorkspaceApp{
// 		{
// 			Slug: "app2",
// 			Healthcheck: coderagentsdk.Healthcheck{
// 				// URL: We don't set the URL for this test because the setup will
// 				// create a httptest server for us and set it for us.
// 				Interval:  1,
// 				Threshold: 1,
// 			},
// 			Health: coderagentsdk.WorkspaceAppHealthInitializing,
// 		},
// 	}
// 	handlers := []http.Handler{
// 		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			httpapi.Write(r.Context(), w, http.StatusInternalServerError, nil)
// 		}),
// 	}
// 	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
// 	defer closeFn()
// 	require.Eventually(t, func() bool {
// 		apps, err := getApps(ctx)
// 		if err != nil {
// 			return false
// 		}
//
// 		return apps[0].Health == coderagentsdk.WorkspaceAppHealthUnhealthy
// 	}, 15*time.Second, 2*time.Second)
// }
//
// func TestAppHealth_Timeout(t *testing.T) {
// 	t.Parallel()
// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()
// 	apps := []coderagentsdk.WorkspaceApp{
// 		{
// 			Slug: "app2",
// 			Healthcheck: coderagentsdk.Healthcheck{
// 				// URL: We don't set the URL for this test because the setup will
// 				// create a httptest server for us and set it for us.
// 				Interval:  1,
// 				Threshold: 1,
// 			},
// 			Health: coderagentsdk.WorkspaceAppHealthInitializing,
// 		},
// 	}
// 	handlers := []http.Handler{
// 		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			// sleep longer than the interval to cause the health check to time out
// 			time.Sleep(2 * time.Second)
// 			httpapi.Write(r.Context(), w, http.StatusOK, nil)
// 		}),
// 	}
// 	getApps, closeFn := setupAppReporter(ctx, t, apps, handlers)
// 	defer closeFn()
// 	require.Eventually(t, func() bool {
// 		apps, err := getApps(ctx)
// 		if err != nil {
// 			return false
// 		}
//
// 		return apps[0].Health == coderagentsdk.WorkspaceAppHealthUnhealthy
// 	}, 15*time.Second, 2*time.Second)
// }
//
// func TestAppHealth_NotSpamming(t *testing.T) {
// 	t.Parallel()
// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()
// 	apps := []coderagentsdk.WorkspaceApp{
// 		{
// 			Slug: "app2",
// 			Healthcheck: coderagentsdk.Healthcheck{
// 				// URL: We don't set the URL for this test because the setup will
// 				// create a httptest server for us and set it for us.
// 				Interval:  1,
// 				Threshold: 1,
// 			},
// 			Health: coderagentsdk.WorkspaceAppHealthInitializing,
// 		},
// 	}
//
// 	counter := new(int32)
// 	handlers := []http.Handler{
// 		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 			atomic.AddInt32(counter, 1)
// 		}),
// 	}
// 	_, closeFn := setupAppReporter(ctx, t, apps, handlers)
// 	defer closeFn()
// 	// Ensure we haven't made more than 2 (expected 1 + 1 for buffer) requests in the last second.
// 	// if there is a bug where we are spamming the healthcheck route this will catch it.
// 	time.Sleep(time.Second)
// 	require.LessOrEqual(t, *counter, int32(2))
// }
//
// func setupAppReporter(ctx context.Context, t *testing.T, apps []coderagentsdk.WorkspaceApp, handlers []http.Handler) (agent.WorkspaceAgentApps, func()) {
// 	closers := []func(){}
// 	for i, handler := range handlers {
// 		if handler == nil {
// 			continue
// 		}
// 		ts := httptest.NewServer(handler)
// 		app := apps[i]
// 		app.Healthcheck.URL = ts.URL
// 		apps[i] = app
// 		closers = append(closers, ts.Close)
// 	}
//
// 	var mu sync.Mutex
// 	workspaceAgentApps := func(context.Context) ([]coderagentsdk.WorkspaceApp, error) {
// 		mu.Lock()
// 		defer mu.Unlock()
// 		var newApps []coderagentsdk.WorkspaceApp
// 		return append(newApps, apps...), nil
// 	}
// 	postWorkspaceAgentAppHealth := func(_ context.Context, req coderagentsdk.PostWorkspaceAppHealthsRequest) error {
// 		mu.Lock()
// 		for id, health := range req.Healths {
// 			for i, app := range apps {
// 				if app.ID != id {
// 					continue
// 				}
// 				app.Health = health
// 				apps[i] = app
// 			}
// 		}
// 		mu.Unlock()
//
// 		return nil
// 	}
//
// 	go agent.NewWorkspaceAppHealthReporter(slogtest.Make(t, nil).Leveled(slog.LevelDebug), apps, postWorkspaceAgentAppHealth)(ctx)
//
// 	return workspaceAgentApps, func() {
// 		for _, closeFn := range closers {
// 			closeFn()
// 		}
// 	}
// }
