part of 'models.dart';

class XrayProfile {
  XrayProfile({
    this.id = '',
    required this.name,
    required this.protocol,
    required this.address,
    required this.port,
    this.rawUri = '',
    this.rawJson = '',
    this.subscriptionId = '',
    this.uuid = '',
    this.password = '',
    this.username = '',
    this.method = '',
    this.alterId = 0,
    this.encryption = '',
    this.flow = '',
    this.network = 'tcp',
    this.headerType = '',
    this.host = '',
    this.path = '',
    this.serviceName = '',
    this.mode = '',
    this.extra = '',
    this.security = '',
    this.sni = '',
    this.alpn = const [],
    this.fingerprint = '',
    this.allowInsecure = false,
    this.publicKey = '',
    this.shortId = '',
    this.spiderX = '',
    this.delayMs = 0,
    this.delayError = '',
    this.testedAt,
    this.addedAt,
  });

  factory XrayProfile.fromJson(Map<String, Object?> j) => XrayProfile(
    id: _str(j['id']),
    name: _str(j['name']),
    protocol: _str(j['protocol']),
    address: _str(j['address']),
    port: _int(j['port']),
    rawUri: _str(j['raw_uri']),
    rawJson: _str(j['raw_json']),
    subscriptionId: _str(j['subscription_id']),
    uuid: _str(j['uuid']),
    password: _str(j['password']),
    username: _str(j['username']),
    method: _str(j['method']),
    alterId: _int(j['alter_id']),
    encryption: _str(j['encryption']),
    flow: _str(j['flow']),
    network: _str(j['network']),
    headerType: _str(j['header_type']),
    host: _str(j['host']),
    path: _str(j['path']),
    serviceName: _str(j['service_name']),
    mode: _str(j['mode']),
    extra: _str(j['extra']),
    security: _str(j['security']),
    sni: _str(j['sni']),
    alpn: [
      for (final a in (j['alpn'] as List<Object?>? ?? const [])) _str(a),
    ],
    fingerprint: _str(j['fingerprint']),
    allowInsecure: _bool(j['allow_insecure']),
    publicKey: _str(j['public_key']),
    shortId: _str(j['short_id']),
    spiderX: _str(j['spider_x']),
    delayMs: _int(j['delay_ms']),
    delayError: _str(j['delay_error']),
    testedAt: _time(j['tested_at']),
    addedAt: _time(j['added_at']),
  );

  final String id;
  final String name;
  final String protocol;
  final String address;
  final int port;
  final String rawUri;
  final String rawJson;
  final String subscriptionId;
  final String uuid;
  final String password;
  final String username;
  final String method;
  final int alterId;
  final String encryption;
  final String flow;
  final String network;
  final String headerType;
  final String host;
  final String path;
  final String serviceName;
  final String mode;
  final String extra;
  final String security;
  final String sni;
  final List<String> alpn;
  final String fingerprint;
  final bool allowInsecure;
  final String publicKey;
  final String shortId;
  final String spiderX;
  final int delayMs;
  final String delayError;
  final DateTime? testedAt;
  final DateTime? addedAt;

  XrayProfile copyWith({
    String? id,
    String? name,
    String? protocol,
    String? address,
    int? port,
    String? rawUri,
    String? rawJson,
    String? subscriptionId,
    String? uuid,
    String? password,
    String? username,
    String? method,
    int? alterId,
    String? encryption,
    String? flow,
    String? network,
    String? headerType,
    String? host,
    String? path,
    String? serviceName,
    String? mode,
    String? extra,
    String? security,
    String? sni,
    List<String>? alpn,
    String? fingerprint,
    bool? allowInsecure,
    String? publicKey,
    String? shortId,
    String? spiderX,
    int? delayMs,
    String? delayError,
  }) => XrayProfile(
    id: id ?? this.id,
    name: name ?? this.name,
    protocol: protocol ?? this.protocol,
    address: address ?? this.address,
    port: port ?? this.port,
    rawUri: rawUri ?? this.rawUri,
    rawJson: rawJson ?? this.rawJson,
    subscriptionId: subscriptionId ?? this.subscriptionId,
    uuid: uuid ?? this.uuid,
    password: password ?? this.password,
    username: username ?? this.username,
    method: method ?? this.method,
    alterId: alterId ?? this.alterId,
    encryption: encryption ?? this.encryption,
    flow: flow ?? this.flow,
    network: network ?? this.network,
    headerType: headerType ?? this.headerType,
    host: host ?? this.host,
    path: path ?? this.path,
    serviceName: serviceName ?? this.serviceName,
    mode: mode ?? this.mode,
    extra: extra ?? this.extra,
    security: security ?? this.security,
    sni: sni ?? this.sni,
    alpn: alpn ?? this.alpn,
    fingerprint: fingerprint ?? this.fingerprint,
    allowInsecure: allowInsecure ?? this.allowInsecure,
    publicKey: publicKey ?? this.publicKey,
    shortId: shortId ?? this.shortId,
    spiderX: spiderX ?? this.spiderX,
    delayMs: delayMs ?? this.delayMs,
    delayError: delayError ?? this.delayError,
    testedAt: testedAt,
    addedAt: addedAt,
  );

  Map<String, Object?> toJson() => {
    if (id.isNotEmpty) 'id': id,
    'name': name,
    'protocol': protocol,
    'address': address,
    'port': port,
    if (rawUri.isNotEmpty) 'raw_uri': rawUri,
    if (rawJson.isNotEmpty) 'raw_json': rawJson,
    if (subscriptionId.isNotEmpty) 'subscription_id': subscriptionId,
    if (uuid.isNotEmpty) 'uuid': uuid,
    if (password.isNotEmpty) 'password': password,
    if (username.isNotEmpty) 'username': username,
    if (method.isNotEmpty) 'method': method,
    if (alterId != 0) 'alter_id': alterId,
    if (encryption.isNotEmpty) 'encryption': encryption,
    if (flow.isNotEmpty) 'flow': flow,
    if (network.isNotEmpty) 'network': network,
    if (headerType.isNotEmpty) 'header_type': headerType,
    if (host.isNotEmpty) 'host': host,
    if (path.isNotEmpty) 'path': path,
    if (serviceName.isNotEmpty) 'service_name': serviceName,
    if (mode.isNotEmpty) 'mode': mode,
    if (extra.isNotEmpty) 'extra': extra,
    if (security.isNotEmpty) 'security': security,
    if (sni.isNotEmpty) 'sni': sni,
    if (alpn.isNotEmpty) 'alpn': alpn,
    if (fingerprint.isNotEmpty) 'fingerprint': fingerprint,
    if (allowInsecure) 'allow_insecure': allowInsecure,
    if (publicKey.isNotEmpty) 'public_key': publicKey,
    if (shortId.isNotEmpty) 'short_id': shortId,
    if (spiderX.isNotEmpty) 'spider_x': spiderX,
  };
}

class XraySubscription {
  XraySubscription.fromJson(Map<String, Object?> j)
    : id = _str(j['id']),
      name = _str(j['name']),
      url = _str(j['url']),
      enabled = j['enabled'] != false,
      autoUpdate = _bool(j['auto_update']),
      updatedAt = _time(j['updated_at']),
      upload = _int(j['upload']),
      download = _int(j['download']),
      total = _int(j['total']),
      expire = _int(j['expire']),
      lastError = _str(j['last_error']),
      profileCount = _int(j['profile_count']);

  final String id;
  final String name;
  final String url;
  final bool enabled;
  final bool autoUpdate;
  final DateTime? updatedAt;
  final int upload;
  final int download;
  final int total;
  final int expire;
  final String lastError;
  final int profileCount;

  int get used => upload + download;
}

class XrayDelayResult {
  XrayDelayResult.fromJson(Map<String, Object?> j)
    : id = _str(j['id']),
      delayMs = _int(j['delay_ms']),
      error = _str(j['error']);

  final String id;
  final int delayMs;
  final String error;
}

class XrayStateData {
  XrayStateData({
    required this.engineMode,
    required this.activeProfileId,
    required this.profiles,
    required this.subscriptions,
  });

  XrayStateData.fromJson(Map<String, Object?> j)
    : engineMode = _str(j['engine_mode']).isEmpty ? 'vpn' : _str(j['engine_mode']),
      activeProfileId = _str(j['active_profile_id']),
      profiles = [
        for (final p in (j['profiles'] as List<Object?>? ?? const []))
          XrayProfile.fromJson(_map(p)),
      ],
      subscriptions = [
        for (final s in (j['subscriptions'] as List<Object?>? ?? const []))
          XraySubscription.fromJson(_map(s)),
      ];

  final String engineMode;
  final String activeProfileId;
  final List<XrayProfile> profiles;
  final List<XraySubscription> subscriptions;
}

class XrayStatusView {
  XrayStatusView.fromJson(Map<String, Object?> j)
    : selectionMode = _str(j['selection_mode']).isEmpty
          ? 'manual'
          : _str(j['selection_mode']),
      autoFailover = j['auto_failover'] == true,
      failoverScope = _str(j['failover_scope']).isEmpty
          ? 'all'
          : _str(j['failover_scope']),
      activeProfile = switch (_mapOrNull(j['active_profile'])) {
        final m? => XrayProfile.fromJson(m),
        null => null,
      },
      profileCount = _int(j['profile_count']),
      subscriptionCount = _int(j['subscription_count']);

  final String selectionMode;
  final bool autoFailover;
  final String failoverScope;
  final XrayProfile? activeProfile;
  final int profileCount;
  final int subscriptionCount;

  bool get isAutoMode => selectionMode == 'auto';
}

class XrayFreeConfigEntry {
  XrayFreeConfigEntry.fromJson(Map<String, Object?> j)
    : id = _str(j['id']),
      name = _str(j['name']),
      description = _str(j['description']),
      kind = _str(j['kind']),
      url = _str(j['url']),
      config = _str(j['config']);

  final String id;
  final String name;
  final String description;
  final String kind;
  final String url;
  final String config;

  Map<String, Object?> toJson() => {
    'id': id,
    'name': name,
    if (description.isNotEmpty) 'description': description,
    'kind': kind,
    if (url.isNotEmpty) 'url': url,
    if (config.isNotEmpty) 'config': config,
  };
}
