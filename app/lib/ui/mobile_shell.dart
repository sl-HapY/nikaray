import 'dart:io';

import 'package:material_ui/material_ui.dart';

import '../app_settings.dart';
import '../core/android_core.dart';
import '../core/core_controller.dart';
import '../l10n/app_localizations.dart';
import 'about_page.dart';
import 'format.dart';
import 'announcement_panel.dart';
import 'accounts_page.dart';
import 'logs_page.dart';
import 'overview_page.dart';
import 'servers_page.dart';
import 'settings_page.dart';
import 'widgets.dart';
import 'xray_configs_page.dart';

/// The phone layout: two tabs, Home and Settings, as phone apps have. The
/// rest (accounts, servers, each group of settings, logs, about) opens as a
/// page of its own with the system's back button.
class MobileShell extends StatefulWidget {
  const MobileShell({
    super.key,
    required this.core,
    required this.settings,
    required this.banner,
    this.noCore,
  });

  final CoreController core;
  final AppSettings settings;

  /// Shown above the page when the core stopped answering.
  final Widget? banner;

  /// Shown in place of the pages until the core first answers.
  final Widget? noCore;

  @override
  State<MobileShell> createState() => _MobileShellState();
}

class _MobileShellState extends State<MobileShell> {
  int _tab = 0;

  @override
  void initState() {
    super.initState();
    AndroidCore.homeRequests.addListener(_openHome);
  }

  void _openHome() => setState(() => _tab = 0);

  @override
  void dispose() {
    AndroidCore.homeRequests.removeListener(_openHome);
    super.dispose();
  }

  /// Opens [page] under a top bar named [title]; it follows the core.
  void _push(String title, Widget Function() page) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (context) => ListenableBuilder(
          listenable: widget.core,
          builder: (context, _) => Scaffold(
            appBar: AppBar(title: Text(title)),
            body: SafeArea(
              top: false,
              child: OwnTopBar(child: _readable(page())),
            ),
          ),
        ),
      ),
    );
  }

  /// [page] no wider than a line can be read on a tablet; the home page
  /// keeps the whole width for its colors and limits its own content.
  static Widget _readable(Widget page) => Center(
    child: ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 720),
      child: page,
    ),
  );

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final core = widget.core;
    final settings = widget.settings;
    void openServers() =>
        _push(l.navServers, () => ServersPage(core: core, settings: settings));
    void openAccounts() => _push(
      l.navAccounts,
      () => AccountsPage(core: core, settings: settings),
    );
    void openXrayConfigs() =>
        _push(l.xrayConfigs, () => XrayConfigsPage(core: core));
    void openConnection() => _push(
      l.sectionConnection,
      () => SettingsPage(
        core: core,
        settings: settings,
        section: SettingsSection.connection,
      ),
    );
    void openUpstreamProxy() => _push(
      l.upstreamProxy,
      () => SettingsPage(
        core: core,
        settings: settings,
        section: SettingsSection.upstreamProxy,
      ),
    );
    final page =
        widget.noCore ??
        (_tab == 0
            ? OverviewPage(
                core: core,
                settings: settings,
                onOpenServers: openServers,
                onOpenAccounts: openAccounts,
                onOpenXrayConfigs: openXrayConfigs,
                onOpenConnection: openConnection,
                onOpenUpstreamProxy: openUpstreamProxy,
                onOpenAbout: () =>
                    _push(l.navAbout, () => AboutPage(core: core)),
                phone: true,
              )
            : _readable(
                _SettingsList(
                  onOpenServers: openServers,
                  onOpenAccounts: openAccounts,
                  onOpenXrayConfigs: openXrayConfigs,
                  push: _push,
                  core: core,
                  settings: settings,
                ),
              ));
    return MandatoryUpdateGate(
      core: core,
      child: PopScope(
        canPop: _tab == 0,
        onPopInvokedWithResult: (didPop, _) {
          if (!didPop) setState(() => _tab = 0);
        },
        child: Scaffold(
        body: SafeArea(
          // The home page's hero reaches under the status bar.
          top: _tab != 0 || widget.noCore != null || widget.banner != null,
          child: AnnouncementOverlay(
            settings: settings,
            onOpenUpdates: () =>
                _push(l.checkUpdates, () => AboutPage(core: core)),
            child: Column(
              children: [
                ?widget.banner,
                Expanded(child: page),
              ],
            ),
          ),
        ),
        bottomNavigationBar: _themed(
          context,
          // The dark home page has its own night colors.
          home:
              _tab == 0 &&
              widget.noCore == null &&
              Theme.of(context).brightness == Brightness.dark,
          child: DecoratedBox(
            decoration: BoxDecoration(
              border: Border(
                top: BorderSide(
                  color: Theme.of(
                    context,
                  ).colorScheme.outlineVariant.withValues(alpha: 0.25),
                ),
              ),
            ),
            child: NavigationBar(
              selectedIndex: _tab,
              onDestinationSelected: (i) => setState(() => _tab = i),
              destinations: [
                NavigationDestination(
                  icon: const Icon(Icons.home_outlined),
                  selectedIcon: const Icon(Icons.home_rounded),
                  label: l.navHome,
                ),
                NavigationDestination(
                  icon: const Icon(Icons.settings_outlined),
                  selectedIcon: const Icon(Icons.settings_rounded),
                  label: l.navSettings,
                ),
              ],
            ),
          ),
        ),
      ),
    ),
    );
  }

  /// The home page's midnight theme around [child] while home shows.
  Widget _themed(
    BuildContext context, {
    required bool home,
    required Widget child,
  }) => home
      ? Theme(data: midnightTheme(Theme.of(context)), child: child)
      : child;
}

/// The Settings tab: what the app manages, the groups of settings, and the
/// app's own pages.
class _SettingsList extends StatefulWidget {
  const _SettingsList({
    required this.onOpenServers,
    required this.onOpenAccounts,
    required this.onOpenXrayConfigs,
    required this.push,
    required this.core,
    required this.settings,
  });

  final VoidCallback onOpenServers;
  final VoidCallback onOpenAccounts;
  final VoidCallback onOpenXrayConfigs;
  final void Function(String title, Widget Function() page) push;
  final CoreController core;
  final AppSettings settings;

  @override
  State<_SettingsList> createState() => _SettingsListState();
}

class _SettingsListState extends State<_SettingsList> {
  /// What was typed, folded for matching; empty shows the whole list.
  String _query = '';
  final _search = TextEditingController();
  late int _settingsTab = widget.core.status?.isXrayMode == true ? 1 : 0;

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final core = widget.core;
    final settings = widget.settings;
    final push = widget.push;
    final onOpenServers = widget.onOpenServers;
    final onOpenAccounts = widget.onOpenAccounts;
    final onOpenXrayConfigs = widget.onOpenXrayConfigs;
    bool matches(String name) =>
        _query.isEmpty || foldForSearch(name).contains(_query);
    // A group keeps only the rows whose name matches, and goes away itself
    // when none of them does.
    (String, List<Widget>)? group(
      String title,
      List<(String, Widget)> tiles,
    ) {
      final kept = [
        for (final (name, tile) in tiles)
          if (matches(name)) tile,
      ];
      return kept.isEmpty ? null : (title, kept);
    }

    Widget tile(IconData icon, String title, VoidCallback onTap) => ListTile(
      leading: Icon(icon),
      title: Text(title),
      // Mirrors itself in right-to-left languages.
      trailing: const Icon(Icons.chevron_right_rounded),
      onTap: onTap,
    );
    void open(SettingsSection section, String name) => push(
      name,
      () => SettingsPage(core: core, settings: settings, section: section),
    );
    (String, Widget) section(
      SettingsSection section,
      IconData icon,
      String name,
    ) => (name, tile(icon, name, () => open(section, name)));
    (String, Widget) row(IconData icon, String name, VoidCallback onTap) =>
        (name, tile(icon, name, onTap));
    // The sections this list opens: TUN is not among them, and the app's
    // port and web dashboard are a computer's.
    final offered = {
      SettingsSection.general,
      SettingsSection.xray,
      SettingsSection.connection,
      SettingsSection.routing,
      SettingsSection.dns,
      SettingsSection.relay,
      SettingsSection.fragment,
      SettingsSection.upstreamProxy,
      if (!Platform.isAndroid) SettingsSection.advanced,
    };
    // The sections' own settings, when something was typed: each one under
    // the section that holds it, so one tap opens it.
    final hits = _query.isEmpty
        ? const <(SettingsSection, String)>[]
        : [
            for (final entry in settingsIndex(l))
              if (foldForSearch(entry.$2).contains(_query) &&
                  offered.contains(entry.$1) &&
                  settingOnThisPlatform(l, entry.$2))
                entry,
          ];
    final showVpn = _query.isNotEmpty || _settingsTab == 0;
    final showXray = _query.isNotEmpty || _settingsTab == 1;
    final groups = [
      // Common settings shared by both VPN and Xray tabs.
      ?group(l.sectionGeneral, [
        section(
          SettingsSection.general,
          Icons.tune_rounded,
          l.sectionGeneral,
        ),
        section(
          SettingsSection.routing,
          Icons.alt_route_rounded,
          l.sectionRouting,
        ),
        section(SettingsSection.dns, Icons.dns_rounded, l.sectionDns),
      ]),
      if (showXray)
        ?group(l.settingsTabXray, [
          row(Icons.hub_outlined, l.xrayConfigs, onOpenXrayConfigs),
          section(
            SettingsSection.xray,
            Icons.tune_rounded,
            l.sectionXray,
          ),
        ]),
      if (showVpn)
        ?group(l.settingsTabVpn, [
          row(Icons.public_rounded, l.navServers, onOpenServers),
          row(Icons.people_outline_rounded, l.navAccounts, onOpenAccounts),
          section(
            SettingsSection.connection,
            Icons.vpn_lock_outlined,
            l.sectionConnection,
          ),
          section(SettingsSection.relay, Icons.cloud_outlined, l.apiRelay),
          section(
            SettingsSection.fragment,
            Icons.content_cut_rounded,
            l.tlsFragment,
          ),
          section(
            SettingsSection.upstreamProxy,
            Icons.lan_outlined,
            l.upstreamProxy,
          ),
        ]),
      // Common app-level settings shared across both tabs.
      ?group(l.groupApp, [
        if (!Platform.isAndroid)
          section(
            SettingsSection.advanced,
            Icons.build_outlined,
            l.sectionAdvanced,
          ),
        row(
          Icons.receipt_long_outlined,
          l.navLogs,
          () => push(l.navLogs, () => LogsPage(core: core)),
        ),
        row(
          Icons.info_outline_rounded,
          l.navAbout,
          () => push(l.navAbout, () => AboutPage(core: core)),
        ),
      ]),
    ];
    final theme = Theme.of(context);
    final unifiedRows = <Widget>[];
    for (var gi = 0; gi < groups.length; gi++) {
      final (title, tiles) = groups[gi];
      if (gi > 0) unifiedRows.add(const Divider(height: 1));
      unifiedRows.add(
        Padding(
          padding: const EdgeInsetsDirectional.fromSTEB(16, 14, 16, 4),
          child: Text(
            title,
            style: theme.textTheme.labelMedium?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.4,
            ),
          ),
        ),
      );
      for (var ti = 0; ti < tiles.length; ti++) {
        if (ti > 0) unifiedRows.add(const Divider(height: 1, indent: 56));
        unifiedRows.add(tiles[ti]);
      }
    }
    if (hits.isNotEmpty) {
      if (unifiedRows.isNotEmpty) unifiedRows.add(const Divider(height: 1));
      unifiedRows.add(
        Padding(
          padding: const EdgeInsetsDirectional.fromSTEB(16, 14, 16, 4),
          child: Text(
            l.settingsTitle,
            style: theme.textTheme.labelMedium?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.4,
            ),
          ),
        ),
      );
      for (var hi = 0; hi < hits.length; hi++) {
        final (section, name) = hits[hi];
        if (hi > 0) unifiedRows.add(const Divider(height: 1, indent: 56));
        unifiedRows.add(
          ListTile(
            leading: const Icon(Icons.chevron_right_rounded),
            title: Text(name),
            subtitle: Text(settingsSectionName(l, section)),
            onTap: () => open(section, settingsSectionName(l, section)),
          ),
        );
      }
    }
    return ListView(
      padding: pagePadding(context),
      children: [
        PageTitle(l.settingsTitle),
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: TextField(
            controller: _search,
            onChanged: (v) => setState(() => _query = foldForSearch(v)),
            textInputAction: TextInputAction.search,
            decoration: InputDecoration(
              isDense: true,
              hintText: l.search,
              prefixIcon: const Icon(Icons.search_rounded),
              suffixIcon: _search.text.isEmpty
                  ? null
                  : IconButton(
                      tooltip: MaterialLocalizations.of(
                        context,
                      ).deleteButtonTooltip,
                      icon: const Icon(Icons.close_rounded),
                      onPressed: () => setState(() {
                        _search.clear();
                        _query = '';
                      }),
                    ),
              border: const OutlineInputBorder(),
            ),
          ),
        ),
        if (_query.isEmpty) ...[
          const SizedBox(height: 12),
          SegmentedButton<int>(
            segments: [
              ButtonSegment<int>(
                value: 0,
                icon: const Icon(Icons.shield_outlined, size: 17),
                label: Text(l.settingsTabVpn),
              ),
              ButtonSegment<int>(
                value: 1,
                icon: const Icon(Icons.hub_outlined, size: 17),
                label: Text(l.settingsTabXray),
              ),
            ],
            selected: {_settingsTab},
            onSelectionChanged: (s) => setState(() => _settingsTab = s.first),
          ),
        ],
        if (unifiedRows.isEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 32),
            child: Text(
              l.settingsNoMatch,
              textAlign: TextAlign.center,
              style: theme.textTheme.bodyMedium?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          )
        else ...[
          const SizedBox(height: 16),
          Card(
            margin: EdgeInsets.zero,
            clipBehavior: Clip.antiAlias,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: unifiedRows,
            ),
          ),
        ],
      ],
    );
  }
}
