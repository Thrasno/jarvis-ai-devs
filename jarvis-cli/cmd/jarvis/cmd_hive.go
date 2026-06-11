package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveui"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/importui"
)

const hiveDaemonDefaultURL = "http://127.0.0.1:7438"

var hiveDaemonURL string

var newHiveImportClient = func(baseURL string) (importui.Client, error) {
	return hiveclient.New(baseURL)
}

var outputIsTTY = func(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

var hiveCmd = &cobra.Command{
	Use:   "hive",
	Short: "Browse Hive governance TUI (live daemon data)",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseURL := hiveDaemonURL
		if baseURL == "" {
			baseURL = resolveHiveDaemonURL()
		}
		return hiveui.RunHiveTUI(cmd.Context(), baseURL)
	},
}

func init() {
	hiveCmd.Flags().StringVar(&hiveDaemonURL, "daemon-url", "", "hive-daemon base URL (overrides HIVE_DAEMON_URL env var)")
	hiveCmd.AddCommand(newHiveImportEngramCmd())
}

type hiveImportEngramOptions struct {
	source    string
	dryRun    bool
	yes       bool
	previewID string
}

func newHiveImportEngramCmd() *cobra.Command {
	opts := hiveImportEngramOptions{}
	cmd := &cobra.Command{
		Use:   "import-engram",
		Short: "Preview or execute a local Engram-to-Hive import",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHiveImportEngram(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview Engram import impact without Hive writes")
	cmd.Flags().StringVar(&opts.source, "source", "", "Engram SQLite database path (defaults are resolved by hive-daemon)")
	cmd.Flags().StringVar(&opts.previewID, "preview-id", "", "fresh preview id from a successful dry-run")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "execute import without interactive confirmation")
	return cmd
}

func runHiveImportEngram(cmd *cobra.Command, opts hiveImportEngramOptions) error {
	if !opts.dryRun {
		if strings.TrimSpace(opts.previewID) == "" {
			return fmt.Errorf("engram import execution requires --preview-id from a successful dry-run")
		}
		if !opts.yes {
			if err := confirmHiveImport(cmd.OutOrStdout(), cmd.InOrStdin()); err != nil {
				return err
			}
		}
	}
	client, err := newHiveImportClient(selectedHiveDaemonURL())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	uiOpts := importui.Options{
		Source:    strings.TrimSpace(opts.source),
		PreviewID: strings.TrimSpace(opts.previewID),
		Out:       out,
		IsTTY:     outputIsTTY(out),
	}
	if opts.dryRun {
		_, err = importui.RunPreview(cmd.Context(), client, uiOpts)
		return err
	}
	_, err = importui.RunExecute(cmd.Context(), client, uiOpts)
	return err
}

func confirmHiveImport(out io.Writer, in io.Reader) error {
	fmt.Fprint(out, "Type IMPORT to continue: ")
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return fmt.Errorf("engram import confirmation required")
	}
	if strings.TrimSpace(response) != "IMPORT" {
		return fmt.Errorf("engram import confirmation did not match IMPORT")
	}
	return nil
}

func selectedHiveDaemonURL() string {
	if hiveDaemonURL != "" {
		return hiveDaemonURL
	}
	return resolveHiveDaemonURL()
}

// resolveHiveDaemonURL returns the hive-daemon base URL using the same env
// resolution as hiveclient.NewFromEnv: HIVE_DAEMON_URL first, then
// HIVE_HTTP_PORT loopback, then the hardcoded default.
func resolveHiveDaemonURL() string {
	if u := strings.TrimSpace(os.Getenv("HIVE_DAEMON_URL")); u != "" {
		return u
	}
	if port := strings.TrimSpace(os.Getenv("HIVE_HTTP_PORT")); port != "" {
		return "http://127.0.0.1:" + port
	}
	return hiveDaemonDefaultURL
}
