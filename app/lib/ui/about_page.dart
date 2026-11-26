import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';

import '../app_info.dart';
import '../core/core_controller.dart';
import '../core/updates.dart';
import '../l10n/app_localizations.dart';
import 'widgets.dart';

/// The versions, the update check, the source link, and the data folder.
class AboutPage extends StatefulWidget {
  const AboutPage({super.key, required this.core});

  final CoreController core;

  @override
  State<AboutPage> createState() => _AboutPageState();
}

class _AboutPageState extends State<AboutPage> {
  bool _checking = false;
  String? _result;
  Release? _release;

  @override
  void initState() {
    super.initState();
    _release = Updates.available.value;
  }

  Future<void> _check() async {
    final l = AppLocalizations.of(context);
    if (githubRepo.isEmpty) {
      setState(() => _result = l.repoNotSet);
      return;
    }
    setState(() {
      _checking = true;
      _result = null;
      _release = null;
    });
    try {
      final release = await checkForUpdate(
        check: UpdateCheck.manual,
        proxy: updateProxy(widget.core.status),
        onAnnouncements: Updates.receiveAnnouncements,
      );
      Updates.found(release, check: UpdateCheck.manual);
      if (!mounted) return;
      setState(() {
        _release = release;
        _result = release == null
            ? l.upToDate
            : release.mandatory
            ? l.updateRequired(release.version)
            : l.updateAvailable(release.version);
      });
    } on Exception catch (e) {
      // No connection, a timeout, or a refused handshake: say so in words,
      // not as the exception's text.
      final network =
          e is SocketException || e is TimeoutException || e is TlsException;
      if (mounted) {
        setState(
          () => _result = network
              ? l.updateCheckNoNetwork
              : l.updateCheckFailed('$e'),
        );
      }
    } finally {
      if (mounted) setState(() => _checking = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final status = widget.core.status;
    final muted = theme.textTheme.bodySmall?.copyWith(
      color: theme.colorScheme.onSurfaceVariant,
    );
    final scheme = theme.colorScheme;
    final github = githubRepo.isEmpty ? '' : 'https://github.com/$githubRepo';
    return ListView(
      padding: pagePadding(context),
      children: [
        PageTitle(l.navAbout),
        const SizedBox(height: 16),
        Container(
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(24),
            gradient: LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: [
                scheme.primary.withValues(alpha: 0.14),
                scheme.tertiary.withValues(alpha: 0.06),
                theme.cardTheme.color ?? scheme.surface,
              ],
            ),
            border: Border.all(
              color: scheme.outlineVariant.withValues(alpha: 0.45),
            ),
          ),
          padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 24),
          child: Column(
            children: [
              Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(24),
                  boxShadow: [
                    BoxShadow(
                      color: scheme.primary.withValues(alpha: 0.22),
                      blurRadius: 24,
                      offset: const Offset(0, 6),
                    ),
                  ],
                ),
                child: const AppLogo(size: 80, radius: 20),
              ),
              const SizedBox(height: 14),
              Text(
                l.appTitle,
                style: theme.textTheme.headlineSmall?.copyWith(
                  fontWeight: FontWeight.w800,
                  letterSpacing: 0.3,
                ),
              ),
              const SizedBox(height: 8),
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 5,
                ),
                decoration: BoxDecoration(
                  color: scheme.primary.withValues(alpha: 0.12),
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(
                    color: scheme.primary.withValues(alpha: 0.25),
                  ),
                ),
                child: Text(
                  'by salist labs',
                  textDirection: TextDirection.ltr,
                  style: theme.textTheme.labelMedium?.copyWith(
                    color: scheme.primary,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.4,
                  ),
                ),
              ),
              const SizedBox(height: 10),
              Text(l.unofficialNote, style: muted, textAlign: TextAlign.center),
            ],
          ),
        ),
        const SizedBox(height: 14),
        Panel(
          child: Column(
            children: [
              _Line(label: l.appVersionLabel, value: appVersion),
              const Divider(height: 24),
              _Line(label: l.coreVersionLabel, value: status?.version ?? '—'),
              const Divider(height: 24),
              // The result and the Download button under the title, so a
              // phone's narrow screen keeps the text readable.
              Row(
                children: [
                  Expanded(
                    child: Text(
                      l.checkUpdates,
                      style: theme.textTheme.titleSmall,
                    ),
                  ),
                  OutlinedButton.icon(
                    onPressed: _checking ? null : _check,
                    icon: _checking
                        ? const SizedBox.square(
                            dimension: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.update_rounded, size: 18),
                    label: Text(_checking ? l.checkingUpdates : l.checkUpdates),
                  ),
                ],
              ),
              if (_result != null) ...[
                const SizedBox(height: 8),
                Align(
                  alignment: AlignmentDirectional.centerStart,
                  child: Text(_result!, style: muted),
                ),
              ],
              if (_release case final release?) ...[
                const SizedBox(height: 10),
                _UpdateAction(core: widget.core, release: release),
              ],
              const Divider(height: 24),
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(l.sourceCode, style: theme.textTheme.titleSmall),
                        const SizedBox(height: 2),
                        github.isEmpty
                            ? Text(l.repoNotSet, style: muted)
                            : Directionality(
                                textDirection: TextDirection.ltr,
                                child: Text(github, style: muted),
                              ),
                      ],
                    ),
                  ),
                  if (github.isNotEmpty)
                    IconButton(
                      tooltip: l.copyAll,
                      onPressed: () {
                        Clipboard.setData(ClipboardData(text: github));
                        ScaffoldMessenger.of(context).showSnackBar(
                          SnackBar(
                            content: Text(l.copied(github)),
                            behavior: SnackBarBehavior.floating,
                            duration: const Duration(seconds: 2),
                          ),
                        );
                      },
                      icon: const Icon(Icons.copy_rounded, size: 18),
                    ),
                  OutlinedButton.icon(
                    onPressed: github.isEmpty
                        ? null
                        : () => openLink(context, github),
                    icon: const Icon(Icons.open_in_new_rounded, size: 18),
                    label: const Text('GitHub'),
                  ),
                ],
              ),
              // The data folder: not on a phone, where it is the app's
              // private storage, which nobody can open.
              if (!Platform.isAndroid &&
                  status != null &&
                  status.dataDir.isNotEmpty) ...[
                const Divider(height: 24),
                Row(
                  children: [
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(l.dataFolder, style: theme.textTheme.titleSmall),
                          const SizedBox(height: 2),
                          Text(
                            status.dataDir,
                            style: muted,
                            textDirection: TextDirection.ltr,
                          ),
                        ],
                      ),
                    ),
                    OutlinedButton.icon(
                      onPressed: () =>
                          runShowingErrors(context, widget.core.openFolder),
                      icon: const Icon(Icons.folder_open_rounded, size: 18),
                      label: Text(l.openFolder),
                    ),
                  ],
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

/// The found version's button: Update where the app installs it itself
/// (Windows), with the download's progress, why it failed, and Try again;
/// else Download.
class _UpdateAction extends StatelessWidget {
  const _UpdateAction({required this.core, required this.release});

  final CoreController core;
  final Release release;

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: Listenable.merge([core, Updates.failure]),
    builder: (context, _) {
      final l = AppLocalizations.of(context);
      final muted = Theme.of(context).textTheme.bodySmall
          ?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant);
      final update = core.status?.update;
      Widget button(String label, IconData icon, VoidCallback? onPressed) =>
          SizedBox(
            width: double.infinity,
            child: FilledButton.icon(
              onPressed: onPressed,
              icon: Icon(icon, size: 18),
              label: Text(label),
            ),
          );
      if (update == null) {
        return release.url.isEmpty
            ? const SizedBox.shrink()
            : button(
                l.download,
                Icons.download_rounded,
                () => openDownload(context, release),
              );
      }
      final mine = update.version == release.version;
      if (mine && (update.downloading || update.installing)) {
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              update.downloading
                  ? l.updateDownloading(release.version)
                  : l.updateInstalling(release.version),
              style: muted,
            ),
            const SizedBox(height: 8),
            LinearProgressIndicator(
              value: update.downloading ? update.progress : null,
            ),
          ],
        );
      }
      final failure =
          Updates.failure.value ??
          (mine && update.failed ? update.error : null);
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (failure != null) ...[
            Tooltip(
              message: failure,
              child: Text(l.updateFailed(release.version), style: muted),
            ),
            const SizedBox(height: 8),
          ],
          button(
            failure != null ? l.tryAgain : l.updateNow,
            failure != null
                ? Icons.refresh_rounded
                : Icons.system_update_alt_rounded,
            () => Updates.update(release),
          ),
        ],
      );
    },
  );
}

class _Line extends StatelessWidget {
  const _Line({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Row(
      children: [
        Expanded(child: Text(label, style: theme.textTheme.titleSmall)),
        Text(
          value,
          style: theme.textTheme.bodyMedium,
          textDirection: TextDirection.ltr,
        ),
      ],
    );
  }
}
