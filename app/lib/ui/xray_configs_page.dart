import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';

import '../core/core_controller.dart';
import '../core/models.dart';
import '../l10n/app_localizations.dart';
import 'format.dart';
import 'widgets.dart';
import 'xray_edit_dialog.dart';
import 'xray_free_dialog.dart';

/// Monochrome manager screen for all Xray configs, protocols, subscriptions,
/// batch/single latency testing, sorting, cleanup, and manual editing.
class XrayConfigsPage extends StatefulWidget {
  const XrayConfigsPage({super.key, required this.core});

  final CoreController core;

  @override
  State<XrayConfigsPage> createState() => _XrayConfigsPageState();
}

class _XrayConfigsPageState extends State<XrayConfigsPage> {
  XrayStateData? _state;
  bool _loading = true;
  bool _testingAll = false;
  int _testedCount = 0;
  int _totalTesting = 0;
  bool _updatingSubs = false;
  final Set<String> _testingIds = {};
  final Set<String> _updatingSubIds = {};
  String _selectedGroup = 'all'; // 'all', 'manual', or subscription ID

  static int _delayRank(int delayMs) {
    if (delayMs > 0) return delayMs;
    if (delayMs == 0) return 1 << 40;
    return 1 << 50;
  }

  static List<XrayProfile> _sortedProfiles(List<XrayProfile> list) =>
      [...list]
        ..sort((a, b) => _delayRank(a.delayMs).compareTo(_delayRank(b.delayMs)));

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final data = await widget.core.call((api) => api.xrayState());
      if (mounted) {
        setState(() {
          _state = XrayStateData(
            engineMode: data.engineMode,
            activeProfileId: data.activeProfileId,
            profiles: _sortedProfiles(data.profiles),
            subscriptions: data.subscriptions,
          );
          _loading = false;
        });
      }
    } catch (_) {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _snack(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  Future<void> _pasteFromClipboard() async {
    final data = await Clipboard.getData('text/plain');
    if (!mounted) return;
    final l = AppLocalizations.of(context);
    final text = data?.text?.trim() ?? '';
    if (text.isEmpty) {
      showError(context, l.xrayImportHint);
      return;
    }
    var count = 0;
    final messenger = ScaffoldMessenger.of(context);
    final importedMsg = l.xrayImportedCount;
    final ok = await runShowingErrors(context, () async {
      count = await widget.core.call((api) => api.importXray(text));
    });
    if (ok && mounted) {
      messenger.showSnackBar(
        SnackBar(
          content: Text(importedMsg(count)),
          duration: const Duration(seconds: 2),
        ),
      );
      await _load();
      await widget.core.refresh();
    }
  }

  Future<void> _activate(String id) async {
    final ok = await runShowingErrors(context, () async {
      await widget.core.call((api) => api.activateXrayProfile(id));
    });
    if (ok && mounted) {
      await _load();
      await widget.core.refresh();
    }
  }

  Future<void> _testOne(XrayProfile p) async {
    setState(() => _testingIds.add(p.id));
    await runShowingErrors(context, () async {
      await widget.core.call((api) => api.testXrayProfiles([p.id]));
    });
    if (mounted) {
      setState(() => _testingIds.remove(p.id));
      await _load();
    }
  }

  Future<void> _testAll(List<XrayProfile> list) async {
    if (_testingAll || list.isEmpty) return;
    setState(() {
      _testingAll = true;
      _testedCount = 0;
      _totalTesting = list.length;
    });
    final ids = [for (final p in list) p.id];
    await runShowingErrors(context, () async {
      await widget.core.call((api) async {
        await for (final res in api.testXrayProfilesStream(ids)) {
          if (!mounted) return;
          setState(() {
            _testedCount++;
            final cur = _state;
            if (cur != null) {
              final updated = [
                for (final p in cur.profiles)
                  p.id == res.id
                      ? p.copyWith(
                          delayMs: res.delayMs,
                          delayError: res.error,
                        )
                      : p,
              ];
              _state = XrayStateData(
                engineMode: cur.engineMode,
                activeProfileId: cur.activeProfileId,
                profiles: _sortedProfiles(updated),
                subscriptions: cur.subscriptions,
              );
            }
          });
        }
      });
    });
    if (mounted) {
      setState(() {
        _testingAll = false;
        _testedCount = 0;
        _totalTesting = 0;
      });
      await _load();
    }
  }

  Future<void> _sortByDelay() async {
    final ok = await runShowingErrors(context, () async {
      await widget.core.call((api) => api.sortXrayProfiles());
    });
    if (ok && mounted) await _load();
  }

  Future<void> _cleanup({
    bool removeFailed = false,
    bool removeDuplicates = false,
  }) async {
    final ok = await runShowingErrors(context, () async {
      await widget.core.call(
        (api) => api.cleanupXrayProfiles(
          removeFailed: removeFailed,
          removeDuplicates: removeDuplicates,
        ),
      );
    });
    if (ok && mounted) {
      await _load();
      await widget.core.refresh();
    }
  }

  Future<void> _updateSub(XraySubscription sub) async {
    final l = AppLocalizations.of(context);
    setState(() => _updatingSubIds.add(sub.id));
    final ok = await runShowingErrors(context, () async {
      await widget.core.call((api) => api.refreshXraySubscription(sub.id));
    });
    if (mounted) {
      setState(() => _updatingSubIds.remove(sub.id));
      if (ok) _snack(l.xraySubUpdated);
      await _load();
      await widget.core.refresh();
    }
  }

  Future<void> _updateAllSubs() async {
    if (_updatingSubs) return;
    final l = AppLocalizations.of(context);
    setState(() => _updatingSubs = true);
    final ok = await runShowingErrors(context, () async {
      await widget.core.call((api) => api.refreshAllXraySubscriptions());
    });
    if (mounted) {
      setState(() => _updatingSubs = false);
      if (ok) _snack(l.xraySubUpdated);
      await _load();
      await widget.core.refresh();
    }
  }

  Future<void> _deleteSub(XraySubscription sub) async {
    final l = AppLocalizations.of(context);
    var keep = false;
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, setState) => AlertDialog(
          title: Text(l.xrayDeleteSub),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(sub.name),
              const SizedBox(height: 8),
              CheckboxListTile(
                contentPadding: EdgeInsets.zero,
                value: keep,
                title: Text(l.xrayKeepProfiles),
                onChanged: (v) => setState(() => keep = v ?? false),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: Text(l.cancel),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: Text(l.remove),
            ),
          ],
        ),
      ),
    );
    if (confirm != true || !mounted) return;
    final ok = await runShowingErrors(context, () async {
      await widget.core.call(
        (api) => api.deleteXraySubscription(sub.id, keepProfiles: keep),
      );
    });
    if (ok && mounted) {
      if (_selectedGroup == sub.id) _selectedGroup = 'all';
      await _load();
      await widget.core.refresh();
    }
  }

  void _showAddSheet() {
    final l = AppLocalizations.of(context);
    showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.content_paste_rounded),
              title: Text(l.xrayAddFromClipboard),
              onTap: () {
                Navigator.pop(ctx);
                _pasteFromClipboard();
              },
            ),
            ListTile(
              leading: const Icon(Icons.code_rounded),
              title: Text(l.xrayImportUriJson),
              onTap: () async {
                Navigator.pop(ctx);
                final count = await showXrayImportDialog(context, widget.core);
                if (count != null && mounted) {
                  _snack(l.xrayImportedCount(count));
                  await _load();
                }
              },
            ),
            ListTile(
              leading: const Icon(Icons.tune_rounded),
              title: Text(l.xrayCreateManual),
              onTap: () async {
                Navigator.pop(ctx);
                final ok = await showXrayManualConfigDialog(
                  context,
                  widget.core,
                );
                if (ok == true && mounted) await _load();
              },
            ),
            ListTile(
              leading: const Icon(Icons.rss_feed_rounded),
              title: Text(l.xrayAddSubscription),
              onTap: () async {
                Navigator.pop(ctx);
                final ok = await showXraySubscriptionDialog(
                  context,
                  widget.core,
                );
                if (ok == true && mounted) await _load();
              },
            ),
            ListTile(
              leading: const Icon(Icons.cloud_download_outlined),
              title: Text(l.xrayFreeConfigs),
              subtitle: Text(l.xrayFreeConfigsHint),
              onTap: () async {
                Navigator.pop(ctx);
                final ok = await showXrayFreeConfigsDialog(
                  context,
                  widget.core,
                );
                if (ok == true && mounted) await _load();
              },
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final data = _state;
    final allProfiles = data?.profiles ?? const <XrayProfile>[];
    final subs = data?.subscriptions ?? const <XraySubscription>[];
    final activeId = data?.activeProfileId ?? '';

    final visibleProfiles = [
      for (final p in allProfiles)
        if (_selectedGroup == 'all' ||
            (_selectedGroup == 'manual' && p.subscriptionId.isEmpty) ||
            p.subscriptionId == _selectedGroup)
          p,
    ];

    return Stack(
      children: [
        ListView(
          padding: pagePadding(context).add(const EdgeInsets.only(bottom: 84)),
          children: [
            Wrap(
              spacing: 8,
              runSpacing: 8,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                FilledButton.tonalIcon(
                  onPressed: _testingAll || visibleProfiles.isEmpty
                      ? null
                      : () => _testAll(visibleProfiles),
                  icon: _testingAll
                      ? const SizedBox.square(
                          dimension: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.bolt_rounded, size: 18),
                  label: Text(
                    _testingAll
                        ? '${l.xrayTesting} ($_testedCount/$_totalTesting)'
                        : l.xrayTestAll,
                    maxLines: 1,
                    softWrap: false,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                OutlinedButton.icon(
                  onPressed: () async {
                    final ok = await showXrayFreeConfigsDialog(
                      context,
                      widget.core,
                    );
                    if (ok == true && mounted) await _load();
                  },
                  icon: const Icon(Icons.cloud_download_outlined, size: 18),
                  label: Text(
                    l.xrayFreeConfigs,
                    maxLines: 1,
                    softWrap: false,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                IconButton.filledTonal(
                  tooltip: l.xraySortDelay,
                  onPressed: allProfiles.length < 2 ? null : _sortByDelay,
                  icon: const Icon(Icons.sort_rounded),
                ),
                PopupMenuButton<String>(
                  icon: const Icon(Icons.more_vert_rounded),
                  onSelected: (v) async {
                    switch (v) {
                      case 'update_subs':
                        await _updateAllSubs();
                      case 'clean_failed':
                        await _cleanup(removeFailed: true);
                      case 'clean_dup':
                        await _cleanup(removeDuplicates: true);
                    }
                  },
                  itemBuilder: (_) => [
                    if (subs.isNotEmpty)
                      PopupMenuItem(
                        value: 'update_subs',
                        child: Text(l.xrayUpdateAllSubs),
                      ),
                    PopupMenuItem(
                      value: 'clean_failed',
                      child: Text(l.xrayCleanFailed),
                    ),
                    PopupMenuItem(
                      value: 'clean_dup',
                      child: Text(l.xrayCleanDuplicates),
                    ),
                  ],
                ),
              ],
            ),
            if (_loading) ...[
              const SizedBox(height: 16),
              const LinearProgressIndicator(),
            ],
            if (subs.isNotEmpty) ...[
              const SizedBox(height: 14),
              Row(
                children: [
                  Text(
                    l.xraySubscriptions,
                    style: theme.textTheme.titleSmall?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const Spacer(),
                  if (_updatingSubs)
                    const SizedBox.square(
                      dimension: 14,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                ],
              ),
              const SizedBox(height: 8),
              for (final sub in subs)
                _SubscriptionCard(
                  sub: sub,
                  updating: _updatingSubIds.contains(sub.id),
                  onRefresh: () => _updateSub(sub),
                  onEdit: () async {
                    final ok = await showXraySubscriptionDialog(
                      context,
                      widget.core,
                      existing: sub,
                    );
                    if (ok == true && mounted) await _load();
                  },
                  onDelete: () => _deleteSub(sub),
                ),
            ],
            const SizedBox(height: 12),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  FilterChip(
                    selected: _selectedGroup == 'all',
                    label: Text('${l.xrayAllConfigs} (${allProfiles.length})'),
                    onSelected: (_) => setState(() => _selectedGroup = 'all'),
                  ),
                  const SizedBox(width: 8),
                  FilterChip(
                    selected: _selectedGroup == 'manual',
                    label: Text(l.xrayManualGroup),
                    onSelected: (_) =>
                        setState(() => _selectedGroup = 'manual'),
                  ),
                  for (final sub in subs) ...[
                    const SizedBox(width: 8),
                    FilterChip(
                      selected: _selectedGroup == sub.id,
                      label: Text('${sub.name} (${sub.profileCount})'),
                      onSelected: (_) =>
                          setState(() => _selectedGroup = sub.id),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 12),
            if (!_loading && visibleProfiles.isEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 32, horizontal: 16),
                child: Column(
                  children: [
                    Icon(
                      Icons.hub_outlined,
                      size: 44,
                      color: cs.onSurfaceVariant,
                    ),
                    const SizedBox(height: 12),
                    Text(
                      l.xrayConfigsEmpty,
                      textAlign: TextAlign.center,
                      style: theme.textTheme.bodyMedium?.copyWith(
                        color: cs.onSurfaceVariant,
                      ),
                    ),
                    const SizedBox(height: 14),
                    FilledButton.icon(
                      onPressed: _showAddSheet,
                      icon: const Icon(Icons.add_rounded),
                      label: Text(l.xrayImportButton),
                    ),
                  ],
                ),
              )
            else
              for (final p in visibleProfiles)
                _ProfileCard(
                  profile: p,
                  active: p.id == activeId,
                  testing: _testingIds.contains(p.id),
                  onSelect: () => _activate(p.id),
                  onTest: () => _testOne(p),
                  onCopy: () {
                    final uri = p.rawUri.isNotEmpty ? p.rawUri : p.rawJson;
                    Clipboard.setData(ClipboardData(text: uri));
                    _snack(l.copied(p.name));
                  },
                  onEdit: () async {
                    final ok = await showXrayManualConfigDialog(
                      context,
                      widget.core,
                      existing: p,
                    );
                    if (ok == true && mounted) await _load();
                  },
                  onDelete: () async {
                    final ok = await runShowingErrors(context, () async {
                      await widget.core.call(
                        (api) => api.deleteXrayProfile(p.id),
                      );
                    });
                    if (ok && mounted) {
                      await _load();
                      await widget.core.refresh();
                    }
                  },
                ),
          ],
        ),
        Positioned(
          right: 16,
          bottom: 16,
          child: FloatingActionButton(
            onPressed: _showAddSheet,
            child: const Icon(Icons.add_rounded),
          ),
        ),
      ],
    );
  }
}

class _SubscriptionCard extends StatelessWidget {
  const _SubscriptionCard({
    required this.sub,
    required this.updating,
    required this.onRefresh,
    required this.onEdit,
    required this.onDelete,
  });

  final XraySubscription sub;
  final bool updating;
  final VoidCallback onRefresh;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final hasUsage = sub.total > 0 || sub.used > 0;
    final frac = sub.total > 0 ? (sub.used / sub.total).clamp(0.0, 1.0) : 0.0;
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsetsDirectional.fromSTEB(14, 10, 6, 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.rss_feed_rounded, size: 18),
                const SizedBox(width: 8),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        sub.name,
                        style: theme.textTheme.titleSmall?.copyWith(
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      Text(
                        l.xrayConfigCount(sub.profileCount),
                        style: theme.textTheme.bodySmall?.copyWith(
                          color: cs.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                IconButton(
                  tooltip: l.xrayUpdateAllSubs,
                  onPressed: updating ? null : onRefresh,
                  icon: updating
                      ? const SizedBox.square(
                          dimension: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.sync_rounded, size: 20),
                ),
                IconButton(
                  tooltip: l.xrayEditConfig,
                  onPressed: onEdit,
                  icon: const Icon(Icons.edit_outlined, size: 19),
                ),
                IconButton(
                  tooltip: l.remove,
                  onPressed: onDelete,
                  icon: const Icon(Icons.delete_outline_rounded, size: 19),
                ),
              ],
            ),
            if (hasUsage) ...[
              const SizedBox(height: 6),
              if (sub.total > 0)
                LinearProgressIndicator(
                  value: frac,
                  minHeight: 4,
                  borderRadius: BorderRadius.circular(2),
                ),
              const SizedBox(height: 4),
              Text(
                sub.total > 0
                    ? '${formatBytes(sub.used)} / ${formatBytes(sub.total)}'
                    : formatBytes(sub.used),
                textDirection: TextDirection.ltr,
                style: theme.textTheme.labelSmall?.copyWith(
                  color: cs.onSurfaceVariant,
                ),
              ),
            ],
            if (sub.lastError.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                sub.lastError,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.labelSmall?.copyWith(color: cs.error),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _ProfileCard extends StatelessWidget {
  const _ProfileCard({
    required this.profile,
    required this.active,
    required this.testing,
    required this.onSelect,
    required this.onTest,
    required this.onCopy,
    required this.onEdit,
    required this.onDelete,
  });

  final XrayProfile profile;
  final bool active;
  final bool testing;
  final VoidCallback onSelect;
  final VoidCallback onTest;
  final VoidCallback onCopy;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final theme = Theme.of(context);
    final cs = theme.colorScheme;

    final tags = <String>[
      profile.protocol.toUpperCase(),
      if (profile.network.isNotEmpty) profile.network.toUpperCase(),
      if (profile.security.isNotEmpty && profile.security != 'none')
        profile.security.toUpperCase(),
      if (profile.flow.isNotEmpty) profile.flow,
    ];

    final delayLabel = testing
        ? '...'
        : profile.delayMs > 0
        ? '${profile.delayMs} ms'
        : profile.delayMs < 0
        ? '×'
        : '—';

    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: active ? cs.primary : cs.outlineVariant.withValues(alpha: 0.6),
          width: active ? 2 : 1,
        ),
      ),
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onSelect,
        child: Padding(
          padding: const EdgeInsetsDirectional.fromSTEB(12, 10, 4, 10),
          child: Row(
            children: [
              Icon(
                active
                    ? Icons.radio_button_checked_rounded
                    : Icons.radio_button_off_rounded,
                size: 20,
                color: active ? cs.primary : cs.onSurfaceVariant,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      profile.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.titleSmall?.copyWith(
                        fontWeight: active ? FontWeight.w800 : FontWeight.w600,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      ltr('${profile.address}:${profile.port} · ${tags.join(' · ')}'),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: cs.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ),
              InkWell(
                borderRadius: BorderRadius.circular(14),
                onTap: testing ? null : onTest,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 5,
                  ),
                  decoration: BoxDecoration(
                    color: cs.surfaceContainerHigh,
                    borderRadius: BorderRadius.circular(14),
                  ),
                  child: Text(
                    ltr(delayLabel),
                    style: theme.textTheme.labelMedium?.copyWith(
                      fontWeight: FontWeight.w700,
                      color: profile.delayMs < 0 ? cs.error : cs.onSurface,
                    ),
                  ),
                ),
              ),
              PopupMenuButton<String>(
                icon: const Icon(Icons.more_vert_rounded, size: 19),
                onSelected: (v) {
                  switch (v) {
                    case 'test':
                      onTest();
                    case 'copy':
                      onCopy();
                    case 'edit':
                      onEdit();
                    case 'delete':
                      onDelete();
                  }
                },
                itemBuilder: (_) => [
                  PopupMenuItem(value: 'test', child: Text(l.xrayTestPing)),
                  PopupMenuItem(value: 'copy', child: Text(l.xrayCopyUri)),
                  if (profile.protocol != 'custom')
                    PopupMenuItem(value: 'edit', child: Text(l.xrayEditConfig)),
                  PopupMenuItem(
                    value: 'delete',
                    child: Text(l.xrayDeleteConfig),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
