<?php

error_reporting(0);
ini_set('display_errors', '0');

header('Content-Type: application/json; charset=utf-8');
date_default_timezone_set('Europe/Moscow');

if ($_SERVER['REQUEST_METHOD'] === 'OPTIONS') {
  http_response_code(204);
  exit;
}

if ($_SERVER['REQUEST_METHOD'] !== 'POST') {
  http_response_code(405);
  echo json_encode(['ok' => false, 'error' => 'method']);
  exit;
}

$config = require __DIR__ . '/config.php';
$token = trim((string) ($config['bot_token'] ?? ''));
$chatId = trim((string) ($config['chat_id'] ?? ''));

if ($token === '' || $chatId === '') {
  http_response_code(500);
  echo json_encode(['ok' => false, 'error' => 'not_configured']);
  exit;
}

$raw = file_get_contents('php://input');
$data = json_decode($raw, true);
if (!is_array($data)) {
  $data = $_POST;
}

if (!is_array($data)) {
  http_response_code(400);
  echo json_encode(['ok' => false, 'error' => 'json']);
  exit;
}

$name = clip_text($data['name'] ?? '', 80);
$phone = clip_text($data['phone'] ?? '', 32);
$platforms = clip_text($data['platforms'] ?? '', 80);
$category = clip_text($data['category'] ?? '', 80);
$city = clip_text($data['city'] ?? '', 80);
$dailyVolume = clip_text($data['dailyVolume'] ?? '', 80);

$phoneDigits = preg_replace('/\D+/', '', $phone);

if ($name === '' || $city === '' || strlen($phoneDigits) !== 11 || $phoneDigits[0] !== '7') {
  http_response_code(400);
  echo json_encode(['ok' => false, 'error' => 'validation']);
  exit;
}

$number = next_lead_number(__DIR__ . '/lead-number.txt');
$text = format_lead_html([
  'name' => $name,
  'phoneDigits' => $phoneDigits,
  'platforms' => $platforms !== '' ? $platforms : 'не указано',
  'category' => $category !== '' ? $category : 'не указано',
  'city' => $city,
  'dailyVolume' => $dailyVolume !== '' ? $dailyVolume : 'не указано',
  'number' => $number,
]);

$payload = http_build_query([
  'chat_id' => $chatId,
  'text' => $text,
  'parse_mode' => 'HTML',
  'disable_web_page_preview' => 'true',
]);

$sent = telegram_post($token, $payload);
$decoded = json_decode((string) $sent['body'], true);

if (empty($decoded['ok'])) {
  $payloadPlain = http_build_query([
    'chat_id' => $chatId,
    'text' => strip_tags(str_replace(array('<b>', '</b>'), '', $text)),
    'disable_web_page_preview' => 'true',
  ]);
  $sent = telegram_post($token, $payloadPlain);
  $decoded = json_decode((string) $sent['body'], true);
}

if (empty($decoded['ok'])) {
  http_response_code(502);
  echo json_encode(['ok' => false, 'error' => 'telegram']);
  exit;
}

echo json_encode(['ok' => true]);

function clip_text($value, $max) {
  $value = trim((string) $value);
  if (function_exists('mb_substr')) {
    return mb_substr($value, 0, $max, 'UTF-8');
  }
  return substr($value, 0, $max);
}

function telegram_post($token, $payload) {
  $path = '/bot' . $token . '/sendMessage';
  $url = 'https://api.telegram.org' . $path;
  $ips = array('', '149.154.167.220', '149.154.167.99', '149.154.167.91');
  $body = '';

  if (function_exists('curl_init')) {
    foreach ($ips as $ip) {
      $ch = curl_init($url);
      $opts = array(
        CURLOPT_POST => true,
        CURLOPT_POSTFIELDS => $payload,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT => 8,
        CURLOPT_CONNECTTIMEOUT => 4,
        CURLOPT_IPRESOLVE => CURL_IPRESOLVE_V4,
        CURLOPT_SSL_VERIFYPEER => false,
        CURLOPT_SSL_VERIFYHOST => 0,
      );
      if ($ip !== '' && defined('CURLOPT_RESOLVE')) {
        $opts[CURLOPT_RESOLVE] = array('api.telegram.org:443:' . $ip);
      }
      curl_setopt_array($ch, $opts);
      $result = curl_exec($ch);
      curl_close($ch);
      $decoded = json_decode((string) $result, true);
      if (!empty($decoded['ok'])) {
        return array('body' => $result);
      }
      if ($result) {
        $body = $result;
      }
    }
  }

  $context = stream_context_create(array(
    'http' => array(
      'method' => 'POST',
      'header' => "Content-Type: application/x-www-form-urlencoded\r\n",
      'content' => $payload,
      'timeout' => 8,
      'ignore_errors' => true,
    ),
    'ssl' => array(
      'verify_peer' => false,
      'verify_peer_name' => false,
    ),
  ));
  $result = @file_get_contents($url, false, $context);
  return array('body' => $result === false ? $body : $result);
}

function h($value) {
  return htmlspecialchars((string) $value, ENT_QUOTES, 'UTF-8');
}

function format_pretty_phone($digits) {
  return sprintf(
    '+7 (%s) %s-%s-%s',
    substr($digits, 1, 3),
    substr($digits, 4, 3),
    substr($digits, 7, 2),
    substr($digits, 9, 2)
  );
}

function next_lead_number($file) {
  $handle = @fopen($file, 'c+');
  if ($handle === false) {
    return 1;
  }
  flock($handle, LOCK_EX);
  $raw = stream_get_contents($handle);
  $number = (int) trim((string) $raw) + 1;
  rewind($handle);
  ftruncate($handle, 0);
  fwrite($handle, (string) $number);
  fflush($handle);
  flock($handle, LOCK_UN);
  fclose($handle);
  return $number;
}

function format_lead_html($lead) {
  $pretty = format_pretty_phone($lead['phoneDigits']);
  $hrefTel = 'tel:+' . $lead['phoneDigits'];
  $when = date('d.m.Y H:i');

  return
    "👤 <b>" . h($lead['name']) . "</b>\n" .
    "📞 +" . h($lead['phoneDigits']) . "\n" .
    "<a href=\"" . h($hrefTel) . "\">" . h($pretty) . "</a>\n" .
    "📦 Объём в день: " . h($lead['dailyVolume']) . "\n\n" .
    "<b>Ответы теста</b>\n" .
    "• На каких площадках вы торгуете\n— <b>" . h($lead['platforms']) . "</b>\n" .
    "• Какая у вас категория товара\n— <b>" . h($lead['category']) . "</b>\n" .
    "• Из какого города отгружаете товар\n— <b>" . h($lead['city']) . "</b>\n" .
    "• Какой объём заказов в день\n— <b>" . h($lead['dailyVolume']) . "</b>\n\n" .
    "🕐 " . h($when) . " · заявка №" . (int) $lead['number'];
}
