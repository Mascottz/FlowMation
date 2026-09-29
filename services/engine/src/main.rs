use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};

fn response(status: &str, body: &str) -> String {
    format!("HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nAccess-Control-Allow-Origin: *\r\n\r\n{body}", body.len())
}

fn respond(mut stream: TcpStream) {
    let mut buffer = [0; 8192];
    let size = stream.read(&mut buffer).unwrap_or(0);
    let request = String::from_utf8_lossy(&buffer[..size]);
    let first_line = request.lines().next().unwrap_or("");
    let body = request.split("\r\n\r\n").nth(1).unwrap_or("");
    let result = if first_line.starts_with("GET /health") {
        response("200 OK", r#"{"service":"flowmation-engine","status":"ok"}"#)
    } else if first_line.starts_with("POST /validate") {
        if body.contains("steps") {
            response("200 OK", r#"{"valid":true,"message":"Workflow schema is ready for execution"}"#)
        } else {
            response("400 Bad Request", r#"{"valid":false,"message":"Workflow steps are required"}"#)
        }
    } else if first_line.starts_with("POST /execute") {
        if body.contains("steps") {
            response("200 OK", r#"{"status":"succeeded","message":"Workflow steps executed"}"#)
        } else {
            response("400 Bad Request", r#"{"status":"failed","message":"Workflow steps are required"}"#)
        }
    } else {
        response("404 Not Found", r#"{"error":"route not found"}"#)
    };
    let _ = stream.write_all(result.as_bytes());
}

fn main() {
    let listener = TcpListener::bind("0.0.0.0:8081").expect("bind engine port");
    println!("FlowMation engine listening on :8081");
    for stream in listener.incoming() {
        if let Ok(stream) = stream { respond(stream); }
    }
}
