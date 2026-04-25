package commands

import (
	"fmt"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/spf13/cobra"
)

const (
	upgradeRepoOwner = "JLugagne"
	upgradeRepoName  = "scaffor"
)

// NewUpgradeCommand creates the `scaffor upgrade` command, which fetches the
// latest scaffor release from GitHub, verifies it against checksums.txt, and
// atomically replaces the current binary.
func NewUpgradeCommand(currentVersion string) *cobra.Command {
	var (
		checkOnly  bool
		targetVer  string
		prerelease bool
	)

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the scaffor binary to the latest release",
		Long: `Upgrade fetches the latest scaffor release from GitHub, verifies the
checksum against checksums.txt, and atomically replaces the current binary.

Pass --check to only report whether a newer version is available.
Pass --version vX.Y.Z to install a specific version (including downgrades).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			repo := selfupdate.NewRepositorySlug(upgradeRepoOwner, upgradeRepoName)

			updater, err := selfupdate.NewUpdater(selfupdate.Config{
				Validator:  &selfupdate.ChecksumValidator{UniqueFilename: "checksums.txt"},
				Prerelease: prerelease,
			})
			if err != nil {
				return fmt.Errorf("init updater: %w", err)
			}

			var (
				rel   *selfupdate.Release
				found bool
			)
			if targetVer != "" {
				rel, found, err = updater.DetectVersion(ctx, repo, targetVer)
			} else {
				rel, found, err = updater.DetectLatest(ctx, repo)
			}
			if err != nil {
				return fmt.Errorf("query releases: %w", err)
			}
			if !found {
				if targetVer != "" {
					return fmt.Errorf("release %s not found for this OS/arch", targetVer)
				}
				return fmt.Errorf("no release found for this OS/arch")
			}

			if checkOnly {
				if targetVer == "" && rel.LessOrEqual(currentVersion) {
					fmt.Printf("scaffor %s is up to date\n", currentVersion)
					return nil
				}
				fmt.Printf("current: %s\nlatest:  v%s\n", currentVersion, rel.Version())
				return nil
			}

			if targetVer == "" && rel.LessOrEqual(currentVersion) {
				fmt.Printf("scaffor %s is already up to date\n", currentVersion)
				return nil
			}

			exe, err := selfupdate.ExecutablePath()
			if err != nil {
				return fmt.Errorf("resolve current executable: %w", err)
			}

			if err := updater.UpdateTo(ctx, rel, exe); err != nil {
				return fmt.Errorf("apply update: %w", err)
			}

			fmt.Printf("upgraded scaffor %s -> v%s\n", currentVersion, rel.Version())
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether a newer version is available")
	cmd.Flags().StringVar(&targetVer, "version", "", "install a specific version (e.g. v0.6.0)")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "include prereleases when detecting the latest version")

	return cmd
}
