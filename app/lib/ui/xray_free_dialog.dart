import 'package:material_ui/material_ui.dart';

import '../core/core_controller.dart';
import '../core/models.dart';
import '../l10n/app_localizations.dart';
import 'widgets.dart';

/// Shows curated free Xray subscriptions and configs loaded from GitHub
/// (`sl-HapY/nikaray/configs.json`), letting the user add any entry by name.
Future<bool?> showXrayFreeConfigsDialog(
  BuildContext context,
  CoreController core,
) => showDialog<bool>(
  context: context,
  builder: (_) => _FreeConfigsDialog(core: core),
);

class _FreeConfigsDialog extends StatefulWidget {
  const _FreeConfigsDialog({required this.core});

  final CoreController core;

  @override
  State<_FreeConfigsDialog> createState() => _FreeConfigsDialogState();
}

class _FreeConfigsDialogState extends State<_FreeConfigsDialog> {
  List<XrayFreeConfigEntry>? _items;
  String? _error;
  bool _loading = true;
  bool _addedAny = false;
  final Set<String> _adding = {};
  final Set<String> _added = {};

  @override
  void initState() {
    super.initState();
    _fetch();
  }

  Future<void> _fetch() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await widget.core.call((api) => api.xrayFreeConfigs());
      if (mounted) {
        setState(() {
          _items = list;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = '$e';
          _loading = false;
        });
      }
    }
  }

  Future<void> _addEntry(XrayFreeConfigEntry entry) async {
    if (_adding.contains(entry.id)) return;
    final l = AppLocalizations.of(context);
    final messenger = ScaffoldMessenger.of(context);
    final addedText = l.xrayFreeAdded(entry.name);
    setState(() => _adding.add(entry.id));
    final ok = await runShowingErrors(context, () async {
      await widget.core.call((api) => api.addXrayFreeConfig(entry));
    });
    if (mounted) {
      setState(() {
        _adding.remove(entry.id);
        if (ok) {
          _added.add(entry.id);
          _addedAny = true;
        }
      });
      if (ok) {
        await widget.core.refresh();
        messenger.showSnackBar(
          SnackBar(
            content: Text(addedText),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final items = _items ?? const <XrayFreeConfigEntry>[];

    return AlertDialog(
      title: Row(
        children: [
          const Icon(Icons.cloud_download_outlined, size: 22),
          const SizedBox(width: 8),
          Expanded(child: Text(l.xrayFreeConfigs)),
          IconButton(
            tooltip: l.xrayUpdateAllSubs,
            onPressed: _loading ? null : _fetch,
            icon: const Icon(Icons.refresh_rounded, size: 20),
          ),
        ],
      ),
      content: SizedBox(
        width: 460,
        height: 360,
        child: _loading
            ? const Center(child: CircularProgressIndicator())
            : _error != null
            ? Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      _error!,
                      textAlign: TextAlign.center,
                      style: theme.textTheme.bodyMedium?.copyWith(
                        color: cs.error,
                      ),
                    ),
                    const SizedBox(height: 12),
                    FilledButton.tonalIcon(
                      onPressed: _fetch,
                      icon: const Icon(Icons.refresh_rounded),
                      label: Text(l.xrayTestPing),
                    ),
                  ],
                ),
              )
            : items.isEmpty
            ? Center(
                child: Text(
                  l.xrayFreeEmpty,
                  textAlign: TextAlign.center,
                  style: theme.textTheme.bodyMedium?.copyWith(
                    color: cs.onSurfaceVariant,
                  ),
                ),
              )
            : ListView.separated(
                itemCount: items.length,
                separatorBuilder: (_, _) => const Divider(height: 1),
                itemBuilder: (ctx, i) {
                  final entry = items[i];
                  final adding = _adding.contains(entry.id);
                  final done = _added.contains(entry.id);
                  return ListTile(
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 4,
                      vertical: 4,
                    ),
                    leading: Icon(
                      entry.kind == 'subscription'
                          ? Icons.rss_feed_rounded
                          : Icons.hub_outlined,
                    ),
                    title: Text(
                      entry.name,
                      style: theme.textTheme.titleSmall?.copyWith(
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    subtitle: Text(
                      [
                        entry.kind == 'subscription'
                            ? l.xraySubscriptions
                            : l.xrayConfigs,
                        if (entry.description.isNotEmpty) entry.description,
                      ].join(' · '),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    trailing: adding
                        ? const SizedBox.square(
                            dimension: 20,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : done
                        ? Icon(Icons.check_circle_rounded, color: cs.primary)
                        : FilledButton.tonal(
                            onPressed: () => _addEntry(entry),
                            child: Text(l.xrayFreeAdd),
                          ),
                  );
                },
              ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context, _addedAny),
          child: Text(l.close),
        ),
      ],
    );
  }
}
