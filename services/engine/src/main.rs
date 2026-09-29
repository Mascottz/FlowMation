use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};

fn respond(mut stream: TcpStream) {
    let mut buffer = [0; 1024];
    let _ = stream.read(&mut buffer);
    let body = r#"{"service":"flowmation-engine","status":"ok"}"#;
    let response = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        body.len(), body
    );
    let _ = stream.write_all(response.as_bytes());
}

fn main() {
    let listener = TcpListener::bind("0.0.0.0:8081").expect("bind engine port");
    println!("FlowMation engine listening on :8081");
    for stream in listener.incoming() {
        if let Ok(stream) = stream {
            respond(stream);
        }
    }
}
